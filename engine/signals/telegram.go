package signals

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	TELEGRAM_PREVIEW            = "https://t.me/s/%s"
	TELEGRAM_PERMALINK          = "https://t.me/"
	TELEGRAM_BEFORE             = "?before="
	TELEGRAM_AT                 = "@"
	TELEGRAM_ID_SEPARATOR       = "/"
	TELEGRAM_INVALID            = "telegram: expected a public channel name"
	TELEGRAM_UNAVAILABLE        = "%s: public web preview unavailable (unknown or private channel, or preview disabled)"
	TELEGRAM_UNAVAILABLE_STATUS = TELEGRAM_UNAVAILABLE + ": %w"
	TELEGRAM_BAD_POST           = "%s: preview post %s has no valid timestamp"
	TELEGRAM_BAD_PAGE           = "%s: could not read preview markup"
	TELEGRAM_DIV_OPEN           = "div"
	TELEGRAM_DIV_CLOSE          = "/div"
	TELEGRAM_LINE_BREAK         = "br"
)

var (
	TELEGRAM_HANDLE  = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	TELEGRAM_HISTORY = regexp.MustCompile(`(?is)<section\b[^>]*\bclass=["'][^"']*\btgme_channel_history\b`)
	TELEGRAM_MESSAGE = regexp.MustCompile(`(?is)<div\b[^>]*\bdata-post=["']([a-z0-9_]+/[0-9]+)["']`)
	TELEGRAM_TEXT    = regexp.MustCompile(`(?is)<div\b[^>]*\bclass=["'][^"']*\btgme_widget_message_text\b[^"']*["'][^>]*>`)
	TELEGRAM_TIME    = regexp.MustCompile(`(?is)<time\b[^>]*\bdatetime=["']([^"']+)["']`)
	TELEGRAM_TAG     = regexp.MustCompile(`(?is)<!--.*?-->|<(/?[a-z][a-z0-9-]*)(?:[^"'>]|"[^"]*"|'[^']*')*>`)
	TELEGRAM_MORE    = regexp.MustCompile(`(?is)\bdata-before=["'][0-9]+["']`)
)

// fetchTelegram reads the latest preview or pages backward until a requested limit is reached.
func fetchTelegram(ctx context.Context, client *http.Client, source Source, limit int, since time.Time, maxPages int) ([]RawPost, error) {
	handle := strings.TrimPrefix(source.Handle, TELEGRAM_AT)
	if !TELEGRAM_HANDLE.MatchString(handle) {
		return nil, errors.New(TELEGRAM_INVALID)
	}
	endpoint := fmt.Sprintf(TELEGRAM_PREVIEW, handle)
	var posts []RawPost
	var previous int64
	for page := 0; page < maxPages; page++ {
		pageURL := endpoint
		if previous > 0 {
			if err := waitForPublicPage(ctx); err != nil {
				return nil, err
			}
			pageURL += TELEGRAM_BEFORE + strconv.FormatInt(previous, 10)
		}
		body, err := readPublicPage(ctx, client, source.ID(), pageURL)
		if err != nil {
			var status *publicStatusError
			if errors.As(err, &status) && (status.code == http.StatusNotFound || status.code == http.StatusForbidden) {
				return nil, fmt.Errorf(TELEGRAM_UNAVAILABLE_STATUS, source.ID(), err)
			}
			return nil, err
		}
		items, more, err := parseTelegramPage(body, source, handle)
		if err != nil {
			return nil, err
		}
		var oldest int64
		var reachedDate bool
		for _, item := range items {
			_, number, _ := strings.Cut(item.PostID, TELEGRAM_ID_SEPARATOR)
			id, _ := strconv.ParseInt(number, 10, 64)
			if previous > 0 && id >= previous {
				continue // a page may overlap the previous one
			}
			if oldest == 0 || id < oldest {
				oldest = id
			}
			if item.PostedAt.Before(since) {
				reachedDate = true
				continue
			}
			posts = append(posts, item)
			if len(posts) == limit {
				return posts, nil
			}
		}
		if !more || oldest == 0 || reachedDate {
			break
		}
		previous = oldest
	}
	return posts, nil
}

func parseTelegramPage(body []byte, source Source, handle string) ([]RawPost, bool, error) {
	if !TELEGRAM_HISTORY.Match(body) {
		return nil, false, fmt.Errorf(TELEGRAM_UNAVAILABLE, source.ID())
	}
	messages := TELEGRAM_MESSAGE.FindAllSubmatchIndex(body, -1)
	var posts []RawPost
	for i, match := range messages {
		id := string(body[match[2]:match[3]])
		channel, number, _ := strings.Cut(id, TELEGRAM_ID_SEPARATOR)
		postNumber, err := strconv.ParseInt(number, 10, 64)
		if !strings.EqualFold(channel, handle) || err != nil || postNumber <= 0 {
			continue
		}
		end := len(body)
		if i+1 < len(messages) {
			end = messages[i+1][0]
		}
		message := body[match[1]:end]
		date := TELEGRAM_TIME.FindSubmatch(message)
		if date == nil {
			return nil, false, fmt.Errorf(TELEGRAM_BAD_POST, source.ID(), id)
		}
		at, err := time.Parse(time.RFC3339, string(date[1]))
		if err != nil {
			return nil, false, fmt.Errorf(TELEGRAM_BAD_POST, source.ID(), id)
		}
		text, ok := telegramText(string(message))
		if !ok {
			return nil, false, fmt.Errorf(TELEGRAM_BAD_PAGE, source.ID())
		}
		posts = append(posts, RawPost{SourceID: source.ID(), PostID: id, PostedAt: at.UTC(),
			URL: TELEGRAM_PERMALINK + id, Text: text})
	}
	if len(messages) > 0 && len(posts) == 0 {
		return nil, false, fmt.Errorf(TELEGRAM_BAD_PAGE, source.ID())
	}
	// The preview presents oldest first; take the newest posts when the limit cuts a page short.
	slices.Reverse(posts)
	return posts, TELEGRAM_MORE.Match(body), nil
}

// telegramText reads the message text div, leaving preview cards, attribution and reactions out.
func telegramText(message string) (string, bool) {
	start := TELEGRAM_TEXT.FindStringIndex(message)
	if start == nil {
		return "", true // a media-only post still has an id and time
	}
	content := message[start[1]:]
	var text strings.Builder
	depth, previous := 1, 0
	for _, tag := range TELEGRAM_TAG.FindAllStringSubmatchIndex(content, -1) {
		text.WriteString(content[previous:tag[0]])
		previous = tag[1]
		if tag[2] == -1 {
			continue // comment
		}
		switch strings.ToLower(content[tag[2]:tag[3]]) {
		case TELEGRAM_DIV_OPEN:
			depth++
		case TELEGRAM_DIV_CLOSE:
			depth--
			if depth == 0 {
				return html.UnescapeString(text.String()), true
			}
		case TELEGRAM_LINE_BREAK:
			text.WriteByte('\n')
		}
	}
	return "", false
}
