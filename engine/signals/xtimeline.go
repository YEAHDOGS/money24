package signals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	// The page X serves for embedded profile timelines. It needs no key and no login.
	//
	// What it returns is NOT the account's latest posts: without a login X sends about a hundred of
	// the account's best-known posts, from any year. It is a sample for a first look at a source,
	// tilted towards whatever went viral. A fair record needs every post, from the paid X API or
	// from posts recorded live as they are made.
	TIMELINE_URL       = "https://syndication.twitter.com/srv/timeline-profile/screen-name/%s"
	TIMELINE_TIMEOUT   = 12 * time.Second
	TIMELINE_MAX_BYTES = 8 << 20
	TIMELINE_AGENT     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"
	X_TIME_LAYOUT      = "Mon Jan 02 15:04:05 -0700 2006"
	X_STATUS_FORMAT    = "https://x.com/%s/status/%s"
)

var (
	X_HANDLE      = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
	TIMELINE_DATA = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)

	ErrNotXHandle = errors.New("not an X handle")
)

// FetchTimeline reads the public sample of an account's posts. See TIMELINE_URL for what the
// sample is and is not. Reposts of other accounts are left out: they are not this source's calls.
func FetchTimeline(ctx context.Context, client *http.Client, handle string) ([]Tweet, error) {
	if !X_HANDLE.MatchString(handle) {
		return nil, ErrNotXHandle
	}
	ctx, cancel := context.WithTimeout(ctx, TIMELINE_TIMEOUT)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(TIMELINE_URL, handle), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", TIMELINE_AGENT)
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("x timeline: HTTP %d (unknown, private or rate-limited account)", res.StatusCode)
	}
	page, err := io.ReadAll(io.LimitReader(res.Body, TIMELINE_MAX_BYTES))
	if err != nil {
		return nil, err
	}
	m := TIMELINE_DATA.FindSubmatch(page)
	if m == nil {
		return nil, errors.New("x timeline: page has no post data")
	}
	var data struct {
		Props struct {
			PageProps struct {
				Timeline struct {
					Entries []struct {
						Content struct {
							Tweet struct {
								ID        string `json:"id_str"`
								CreatedAt string `json:"created_at"`
								Text      string `json:"full_text"`
								User      struct {
									Handle string `json:"screen_name"`
								} `json:"user"`
							} `json:"tweet"`
						} `json:"content"`
					} `json:"entries"`
				} `json:"timeline"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal(m[1], &data); err != nil {
		return nil, fmt.Errorf("x timeline: %w", err)
	}
	out := []Tweet{}
	for _, e := range data.Props.PageProps.Timeline.Entries {
		t := e.Content.Tweet
		at, err := time.Parse(X_TIME_LAYOUT, t.CreatedAt)
		if err != nil || t.ID == "" || t.Text == "" || (t.User.Handle != "" && !strings.EqualFold(t.User.Handle, handle)) {
			continue
		}
		out = append(out, Tweet{Handle: handle, ID: t.ID, At: at.UTC(), Text: html.UnescapeString(t.Text), URL: fmt.Sprintf(X_STATUS_FORMAT, handle, t.ID)})
	}
	return out, nil
}
