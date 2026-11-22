package signals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DISCORD_API            = "https://discord.com/api/v10/channels/"
	DISCORD_JUMP           = "https://discord.com/channels/%s/%s/%s"
	DISCORD_MESSAGES       = "/messages"
	DISCORD_TOKEN_ENV      = "DISCORD_BOT_TOKEN"
	DISCORD_AUTH           = "Authorization"
	DISCORD_BOT            = "Bot "
	DISCORD_LIMIT          = "limit"
	DISCORD_BEFORE         = "before"
	DISCORD_AFTER          = "after"
	DISCORD_PAGE_SIZE      = 100
	DISCORD_INTERVAL       = 15 * time.Second
	DISCORD_RATE_ATTEMPTS  = 3
	DISCORD_EPOCH_MS       = 1420070400000
	DISCORD_TIME_SHIFT     = 22
	DISCORD_REMAINING      = "X-RateLimit-Remaining"
	DISCORD_RESET_AFTER    = "X-RateLimit-Reset-After"
	DISCORD_RESET          = "X-RateLimit-Reset"
	DISCORD_GLOBAL         = "X-RateLimit-Global"
	DISCORD_SCOPE          = "X-RateLimit-Scope"
	DISCORD_GLOBAL_SCOPE   = "global"
	DISCORD_NORMAL_MESSAGE = 0
	DISCORD_REPLY_MESSAGE  = 19
	DISCORD_EMBED_FIELD    = ": "
	DISCORD_EMPTY          = "Discord returned empty message content and embeds. Check Message Content Intent in Developer Portal > Bot"
	DISCORD_INVALID        = "Discord source requires a numeric channel ID"
	DISCORD_BAD_MESSAGE    = "Discord returned a message without a valid ID or timestamp"
	DISCORD_BAD_CHANNEL    = "Discord source requires a server channel"
	DISCORD_REQUEST_FAILED = "Discord request failed"
	DISCORD_BAD_RESPONSE   = "Discord returned an invalid or oversized response"
	DISCORD_NO_TOKEN       = "Discord bot token is missing from %s"
	DISCORD_HTTP           = "Discord HTTP %d"
	DISCORD_RATE_LIMIT     = "Discord HTTP 429, retry in %s"
)

var errDiscordEmpty = errors.New(DISCORD_EMPTY)

type discordRateLimitError struct {
	delay time.Duration
}

func (e *discordRateLimitError) Error() string {
	return fmt.Sprintf(DISCORD_RATE_LIMIT, e.delay)
}

type discordReader struct {
	client   *http.Client
	interval time.Duration
	mu       sync.Mutex
	channels map[uint64]*discordChannel
	global   map[string]time.Time
}

type discordChannel struct {
	busy  chan struct{}
	next  time.Time
	guild string
}

func newDiscordReader(client *http.Client) *discordReader {
	botClient := *client
	botClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &discordReader{client: &botClient, interval: DISCORD_INTERVAL,
		channels: make(map[uint64]*discordChannel), global: make(map[string]time.Time)}
}

type discordMessage struct {
	ID          string            `json:"id"`
	GuildID     string            `json:"guild_id"`
	Time        time.Time         `json:"timestamp"`
	Content     string            `json:"content"`
	Type        int               `json:"type"`
	Attachments []json.RawMessage `json:"attachments"`
	Stickers    []json.RawMessage `json:"sticker_items"`
	Author      struct {
		Username string `json:"username"`
	} `json:"author"`
	Embeds []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Author      struct {
			Name string `json:"name"`
		} `json:"author"`
		Footer struct {
			Text string `json:"text"`
		} `json:"footer"`
		Fields []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"fields"`
	} `json:"embeds"`
}

func (m discordMessage) text() string {
	parts := []string{m.Content}
	for _, embed := range m.Embeds {
		parts = append(parts, embed.Author.Name, embed.Title, embed.Description)
		for _, field := range embed.Fields {
			if strings.TrimSpace(field.Name) != "" && strings.TrimSpace(field.Value) != "" {
				parts = append(parts, field.Name+DISCORD_EMBED_FIELD+field.Value)
			} else {
				parts = append(parts, field.Name, field.Value)
			}
		}
		parts = append(parts, embed.Footer.Text)
	}
	parts = slices.DeleteFunc(parts, func(part string) bool {
		return strings.TrimSpace(part) == ""
	})
	return strings.Join(parts, "\n")
}

func (r *discordReader) channel(id uint64) *discordChannel {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ch := r.channels[id]; ch != nil {
		return ch
	}
	ch := &discordChannel{busy: make(chan struct{}, 1)}
	r.channels[id] = ch
	return ch
}

func (r *discordReader) request(ctx context.Context, token, endpoint string, ch *discordChannel, into any) error {
	var err error
	for range DISCORD_RATE_ATTEMPTS {
		err = r.requestOnce(ctx, token, endpoint, ch, into)
		var limited *discordRateLimitError
		if !errors.As(err, &limited) {
			return err
		}
	}
	return err
}

func (r *discordReader) requestOnce(ctx context.Context, token, endpoint string, ch *discordChannel, into any) error {
	for {
		r.mu.Lock()
		global := r.global[token]
		r.mu.Unlock()
		wait := max(time.Until(ch.next), time.Until(global))
		if wait <= 0 {
			break
		}
		if err := waitForReader(ctx, wait); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, PUBLIC_REQUEST_TIMEOUT)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New(DISCORD_REQUEST_FAILED)
	}
	req.Header.Set(DISCORD_AUTH, DISCORD_BOT+token)
	req.Header.Set(PUBLIC_USER_AGENT_HEADER, PUBLIC_READER_AGENT)
	ch.next = time.Now().Add(r.interval)
	res, err := r.client.Do(req)
	if err != nil {
		return errors.New(DISCORD_REQUEST_FAILED)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		var limit struct {
			RetryAfter float64 `json:"retry_after"`
			Global     bool    `json:"global"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(nil, res.Body, PUBLIC_PAGE_BYTES)).Decode(&limit)
		wait := max(r.interval, discordRateDelay(res.Header, time.Now()), discordSeconds(limit.RetryAfter))
		until := time.Now().Add(wait)
		ch.next = until
		if limit.Global || res.Header.Get(DISCORD_GLOBAL) == "true" || res.Header.Get(DISCORD_SCOPE) == DISCORD_GLOBAL_SCOPE {
			r.mu.Lock()
			if until.After(r.global[token]) {
				r.global[token] = until
			}
			r.mu.Unlock()
		}
		return &discordRateLimitError{delay: wait}
	}
	if res.Header.Get(DISCORD_REMAINING) == "0" {
		until := time.Now().Add(discordRateDelay(res.Header, time.Now()))
		if until.After(ch.next) {
			ch.next = until
		}
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf(DISCORD_HTTP, res.StatusCode)
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, res.Body, PUBLIC_PAGE_BYTES))
	if err := decoder.Decode(into); err != nil {
		return errors.New(DISCORD_BAD_RESPONSE)
	}
	return nil
}

func discordRateDelay(headers http.Header, now time.Time) time.Duration {
	retry := headers.Get(PUBLIC_RETRY_AFTER_HEADER)
	seconds, _ := strconv.ParseFloat(retry, 64)
	reset, err := strconv.ParseFloat(headers.Get(DISCORD_RESET_AFTER), 64)
	if err != nil {
		epoch, _ := strconv.ParseFloat(headers.Get(DISCORD_RESET), 64)
		reset = epoch - float64(now.UnixMilli())/float64(time.Second/time.Millisecond)
	}
	delay := max(discordSeconds(seconds), discordSeconds(reset))
	if date, err := http.ParseTime(retry); err == nil {
		delay = max(delay, date.Sub(now))
	}
	return delay
}

func discordSeconds(seconds float64) time.Duration {
	if seconds > 0 && seconds < float64(time.Duration(1<<63-1)/time.Second) {
		return time.Duration(seconds * float64(time.Second))
	}
	return 0
}

func (r *discordReader) page(ctx context.Context, source Source, direction string, cursor uint64, limit int) ([]discordMessage, string, error) {
	id, err := strconv.ParseUint(source.Handle, 10, 64)
	if err != nil || id == 0 {
		return nil, "", errors.New(DISCORD_INVALID)
	}
	env := source.TokenEnv
	if env == "" {
		env = DISCORD_TOKEN_ENV
	}
	token := strings.TrimSpace(os.Getenv(env))
	if token == "" {
		return nil, "", fmt.Errorf(DISCORD_NO_TOKEN, env)
	}
	ch := r.channel(id)
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case ch.busy <- struct{}{}:
		defer func() { <-ch.busy }()
	}
	channelURL := DISCORD_API + strconv.FormatUint(id, 10)
	query := url.Values{DISCORD_LIMIT: {strconv.Itoa(limit)}}
	if cursor != 0 {
		query.Set(direction, strconv.FormatUint(cursor, 10))
	}
	var messages []discordMessage
	if err := r.request(ctx, token, channelURL+DISCORD_MESSAGES+"?"+query.Encode(), ch, &messages); err != nil {
		return nil, "", err
	}
	if len(messages) == 0 || ch.guild != "" {
		return messages, ch.guild, nil
	}
	for _, message := range messages {
		if message.GuildID != "" {
			ch.guild = message.GuildID
			return messages, ch.guild, nil
		}
	}
	// Get Channel Messages can omit guild_id. Resolve the channel once for jump links.
	var channel struct {
		GuildID string `json:"guild_id"`
	}
	if err := r.request(ctx, token, channelURL, ch, &channel); err != nil {
		return nil, "", err
	}
	if channel.GuildID == "" {
		return nil, "", errors.New(DISCORD_BAD_CHANNEL)
	}
	ch.guild = channel.GuildID
	return messages, ch.guild, nil
}

// fetch walks back through history or pulls messages after the live cursor. A full
// live page is walked backward to the previous cursor before advancing it.
func (r *discordReader) fetch(ctx context.Context, source Source, before, after uint64, limit int, since time.Time, pages int) ([]RawPost, uint64, error) {
	var posts []RawPost
	seen := make(map[uint64]bool)
	newest := after
	missingText := false
	for page := 0; page < pages && len(posts) < limit; page++ {
		direction, cursor := DISCORD_BEFORE, before
		if page == 0 && after > 0 {
			direction, cursor = DISCORD_AFTER, after
		}
		size := min(DISCORD_PAGE_SIZE, limit-len(posts))
		items, guild, err := r.page(ctx, source, direction, cursor, size)
		if err != nil {
			return nil, after, err
		}
		var oldest uint64
		reachedStart := false
		for _, item := range items {
			id, err := strconv.ParseUint(item.ID, 10, 64)
			if err != nil || id == 0 || item.Time.IsZero() {
				return nil, after, errors.New(DISCORD_BAD_MESSAGE)
			}
			if id <= after || (!since.IsZero() && item.Time.Before(since)) {
				reachedStart = true
				continue
			}
			if before > 0 && id >= before {
				continue
			}
			if oldest == 0 || id < oldest {
				oldest = id
			}
			newest = max(newest, id)
			if seen[id] {
				continue
			}
			seen[id] = true
			body := item.text()
			if body == "" {
				if (item.Type == DISCORD_NORMAL_MESSAGE || item.Type == DISCORD_REPLY_MESSAGE) &&
					len(item.Attachments) == 0 && len(item.Stickers) == 0 {
					missingText = true
				}
				continue
			}
			posts = append(posts, RawPost{SourceID: source.ID(), PostID: item.ID, PostedAt: item.Time.UTC(),
				Author: item.Author.Username, URL: fmt.Sprintf(DISCORD_JUMP, guild, source.Handle, item.ID), Text: body})
		}
		if oldest == 0 || reachedStart || len(items) < size {
			break
		}
		before = oldest
	}
	slices.SortFunc(posts, func(a, b RawPost) int {
		if a.PostedAt.Equal(b.PostedAt) {
			if len(a.PostID) == len(b.PostID) {
				return strings.Compare(b.PostID, a.PostID)
			}
			return len(b.PostID) - len(a.PostID)
		}
		return b.PostedAt.Compare(a.PostedAt)
	})
	if len(posts) == 0 && missingText {
		return nil, newest, errDiscordEmpty
	}
	return posts[:min(limit, len(posts))], newest, nil
}

func discordCursor(at time.Time) uint64 {
	return uint64(at.UnixMilli()-DISCORD_EPOCH_MS) << DISCORD_TIME_SHIFT
}
