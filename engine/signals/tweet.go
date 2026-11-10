package signals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	OEMBED_URL       = "https://publish.twitter.com/oembed"
	OEMBED_TIMEOUT   = 5 * time.Second
	OEMBED_MAX_BYTES = 64 << 10
	// Twitter snowflake ids carry their creation time: (id >> 22) + this epoch, in milliseconds.
	SNOWFLAKE_EPOCH_MS = 1288834974657
	SNOWFLAKE_SHIFT    = 22
)

var (
	X_STATUS_URL = regexp.MustCompile(`^https?://(?:www\.|mobile\.)?(?:x|twitter)\.com/([A-Za-z0-9_]{1,15})/status(?:es)?/(\d{5,25})`)
	TWEET_TEXT   = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	LINE_BREAK   = regexp.MustCompile(`(?i)<br\s*/?>`)
	HTML_TAG     = regexp.MustCompile(`<[^>]+>`)

	ErrNotXStatus = errors.New("not an x.com status link")
)

// Tweet is a shared post resolved to its author, id, time and text.
type Tweet struct {
	Handle string
	ID     string
	At     time.Time
	Text   string
	URL    string
}

// ParseXStatusURL reads the handle, id and posting time out of an x.com or twitter.com status link.
// The time comes from the id itself, so it needs no network.
func ParseXStatusURL(raw string) (Tweet, error) {
	m := X_STATUS_URL.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return Tweet{}, ErrNotXStatus
	}
	id, err := strconv.ParseUint(m[2], 10, 64)
	if err != nil {
		return Tweet{}, ErrNotXStatus
	}
	at := time.UnixMilli(int64(id>>SNOWFLAKE_SHIFT) + SNOWFLAKE_EPOCH_MS).UTC()
	return Tweet{Handle: m[1], ID: m[2], At: at, URL: m[0]}, nil
}

// FindXStatusURL picks the first status link out of shared text ("Check this https://x.com/a/status/1 ...").
func FindXStatusURL(shared string) string {
	for _, field := range strings.Fields(shared) {
		if X_STATUS_URL.MatchString(field) {
			return field
		}
	}
	return ""
}

// tweetTextFromEmbed pulls the post text out of oEmbed HTML: the first <p>, tags stripped, entities decoded.
func tweetTextFromEmbed(embed string) string {
	m := TWEET_TEXT.FindStringSubmatch(embed)
	if m == nil {
		return ""
	}
	text := LINE_BREAK.ReplaceAllString(m[1], "\n")
	return strings.TrimSpace(html.UnescapeString(HTML_TAG.ReplaceAllString(text, "")))
}

// ResolveTweet fetches a public post's text through X's oEmbed endpoint, which needs no API key.
func ResolveTweet(ctx context.Context, client *http.Client, statusURL string) (Tweet, error) {
	t, err := ParseXStatusURL(statusURL)
	if err != nil {
		return t, err
	}
	ctx, cancel := context.WithTimeout(ctx, OEMBED_TIMEOUT)
	defer cancel()
	q := url.Values{"url": {t.URL}, "omit_script": {"1"}, "dnt": {"true"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, OEMBED_URL+"?"+q.Encode(), nil)
	if err != nil {
		return t, err
	}
	res, err := client.Do(req)
	if err != nil {
		return t, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return t, fmt.Errorf("oembed: HTTP %d (deleted, private or rate-limited post)", res.StatusCode)
	}
	var body struct {
		HTML string `json:"html"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, OEMBED_MAX_BYTES)).Decode(&body); err != nil {
		return t, fmt.Errorf("oembed: %w", err)
	}
	t.Text = tweetTextFromEmbed(body.HTML)
	if t.Text == "" {
		return t, errors.New("oembed: no post text")
	}
	return t, nil
}
