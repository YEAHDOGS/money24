package signals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	REDDIT_ORIGIN          = "https://www.reddit.com"
	REDDIT_USER_PATH       = "/user/%s/"
	REDDIT_SUB_PATH        = "/r/%s/"
	REDDIT_SUBMITTED       = "submitted.json"
	REDDIT_COMMENTS        = "comments.json"
	REDDIT_NEW             = "new.json"
	REDDIT_USER            = "u"
	REDDIT_SUB             = "r"
	REDDIT_PATH_SEPARATOR  = "/"
	REDDIT_SUB_PERMALINK   = "/r/"
	REDDIT_USER_PERMALINK  = "/user/"
	REDDIT_QUERY_SEPARATOR = "?"
	REDDIT_AFTER           = "after"
	REDDIT_LIMIT           = "limit"
	REDDIT_RAW_JSON        = "raw_json"
	REDDIT_RAW_VALUE       = "1"
	REDDIT_SORT            = "sort"
	REDDIT_SORT_NEW        = "new"
	REDDIT_LISTING         = "Listing"
	REDDIT_COMMENT         = "t1"
	REDDIT_SUBMISSION      = "t3"
	REDDIT_PAGE_SIZE       = 100
	REDDIT_BACKOFF         = time.Minute
	REDDIT_INVALID         = "reddit: expected u/name or r/sub"
	REDDIT_BAD_RESPONSE    = "%s: expected a Reddit listing"
	REDDIT_BAD_CHILD       = "%s: Reddit listing contains a post without an id, time or permalink"
	REDDIT_BLOCKED         = "%w (Reddit may require OAuth or restrict this listing)"
	REDDIT_COOLDOWN        = "%s: Reddit HTTP 429; retry after %s"
	REDDIT_TITLE_SEPARATOR = "\n\n"
)

var REDDIT_HANDLE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var redditRateLimit struct {
	sync.Mutex
	until time.Time
}

type redditListing struct {
	Kind string `json:"kind"`
	Data *struct {
		After    string `json:"after"`
		Children []struct {
			Kind string `json:"kind"`
			Data struct {
				Name       string  `json:"name"`
				CreatedUTC float64 `json:"created_utc"`
				Permalink  string  `json:"permalink"`
				Title      string  `json:"title"`
				SelfText   string  `json:"selftext"`
				Body       string  `json:"body"`
				LinkID     string  `json:"link_id"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// fetchReddit reads public listings, merging a user's submissions and comments by posting time.
func fetchReddit(ctx context.Context, client *http.Client, source Source, limit int, since time.Time, maxPages int) ([]RawPost, error) {
	kind, name, _ := strings.Cut(source.Handle, REDDIT_PATH_SEPARATOR)
	if !REDDIT_HANDLE.MatchString(name) || (kind != REDDIT_USER && kind != REDDIT_SUB) {
		return nil, errors.New(REDDIT_INVALID)
	}

	path := fmt.Sprintf(REDDIT_SUB_PATH, name)
	listings := []string{REDDIT_NEW}
	if kind == REDDIT_USER {
		path = fmt.Sprintf(REDDIT_USER_PATH, name)
		listings = []string{REDDIT_SUBMITTED, REDDIT_COMMENTS}
	}
	var posts []RawPost
	for i, listing := range listings {
		if i > 0 {
			if err := waitForPublicPage(ctx); err != nil {
				return nil, err
			}
		}
		items, err := fetchRedditListing(ctx, client, source, REDDIT_ORIGIN+path+listing, since, limit, maxPages)
		if err != nil {
			return nil, err
		}
		posts = append(posts, items...)
	}
	slices.SortStableFunc(posts, func(a, b RawPost) int { return b.PostedAt.Compare(a.PostedAt) })
	return posts[:min(limit, len(posts))], nil
}

func fetchRedditListing(ctx context.Context, client *http.Client, source Source, endpoint string, since time.Time, limit, maxPages int) ([]RawPost, error) {
	var posts []RawPost
	var after string
	cursors := map[string]bool{}
	seen := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		if page > 0 {
			if err := waitForPublicPage(ctx); err != nil {
				return nil, err
			}
		}
		query := url.Values{
			REDDIT_LIMIT:    {strconv.Itoa(min(REDDIT_PAGE_SIZE, limit-len(posts)))},
			REDDIT_RAW_JSON: {REDDIT_RAW_VALUE},
			REDDIT_SORT:     {REDDIT_SORT_NEW},
		}
		if after != "" {
			query.Set(REDDIT_AFTER, after)
		}
		body, err := readRedditPage(ctx, client, source.ID(), endpoint+REDDIT_QUERY_SEPARATOR+query.Encode())
		if err != nil {
			return nil, err
		}
		items, next, err := parseRedditPage(body, source)
		if err != nil {
			return nil, err
		}
		var reachedDate bool
		for _, item := range items {
			if item.PostedAt.Before(since) {
				reachedDate = true
				continue
			}
			if seen[item.PostID] {
				continue
			}
			seen[item.PostID] = true
			posts = append(posts, item)
			if len(posts) == limit {
				return posts, nil
			}
		}
		if next == "" || cursors[next] || reachedDate {
			break
		}
		after = next
		cursors[next] = true
	}
	return posts, nil
}

func readRedditPage(ctx context.Context, client *http.Client, source, endpoint string) ([]byte, error) {
	redditRateLimit.Lock()
	until := redditRateLimit.until
	redditRateLimit.Unlock()
	if time.Now().Before(until) {
		return nil, fmt.Errorf(REDDIT_COOLDOWN, source, until.UTC().Format(time.RFC3339))
	}
	body, err := readPublicPage(ctx, client, source, endpoint)
	var status *publicStatusError
	if !errors.As(err, &status) {
		return body, err
	}
	switch status.code {
	case http.StatusTooManyRequests:
		now := time.Now()
		until = now.Add(REDDIT_BACKOFF)
		if seconds, err := strconv.ParseUint(status.retryAfter, 10, 32); err == nil {
			until = now.Add(max(REDDIT_BACKOFF, time.Duration(seconds)*time.Second))
		} else if date, err := http.ParseTime(status.retryAfter); err == nil && date.After(until) {
			until = date
		}
		redditRateLimit.Lock()
		if until.After(redditRateLimit.until) {
			redditRateLimit.until = until
		}
		redditRateLimit.Unlock()
		return nil, fmt.Errorf(REDDIT_COOLDOWN, source, until.UTC().Format(time.RFC3339))
	case http.StatusForbidden, http.StatusUnauthorized:
		return nil, fmt.Errorf(REDDIT_BLOCKED, status)
	default:
		return nil, err
	}
}

func parseRedditPage(body []byte, source Source) ([]RawPost, string, error) {
	var listing redditListing
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, "", fmt.Errorf(PUBLIC_REQUEST_ERROR, source.ID(), err)
	}
	if listing.Kind != REDDIT_LISTING || listing.Data == nil {
		return nil, "", fmt.Errorf(REDDIT_BAD_RESPONSE, source.ID())
	}
	var posts []RawPost
	for _, child := range listing.Data.Children {
		if child.Kind != REDDIT_COMMENT && child.Kind != REDDIT_SUBMISSION {
			continue
		}
		data := child.Data
		validPermalink := strings.HasPrefix(data.Permalink, REDDIT_SUB_PERMALINK) ||
			strings.HasPrefix(data.Permalink, REDDIT_USER_PERMALINK)
		if data.Name == "" || data.CreatedUTC <= 0 || !validPermalink {
			return nil, "", fmt.Errorf(REDDIT_BAD_CHILD, source.ID())
		}
		text := data.Body
		if child.Kind == REDDIT_SUBMISSION {
			text = data.Title
			if data.SelfText != "" {
				text += REDDIT_TITLE_SEPARATOR + data.SelfText
			}
		}
		posts = append(posts, RawPost{SourceID: source.ID(), PostID: data.Name, ThreadID: data.LinkID,
			PostedAt: time.Unix(int64(data.CreatedUTC), 0).UTC(), URL: REDDIT_ORIGIN + data.Permalink, Text: text})
	}
	return posts, listing.Data.After, nil
}
