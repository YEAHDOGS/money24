package signals

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	PUBLIC_READER_AGENT       = "Money24/1.0 (public signal reader; https://github.com/YEAHDOGS/money24)"
	PUBLIC_REQUEST_TIMEOUT    = 12 * time.Second
	PUBLIC_PAGE_BYTES         = 8 << 20
	PUBLIC_PAGE_DELAY         = time.Second
	PUBLIC_HISTORY_POSTS      = 200
	PUBLIC_HISTORY_DAYS       = 30 * 24 * time.Hour
	PUBLIC_HISTORY_PAGES      = 12
	PUBLIC_USER_AGENT_HEADER  = "User-Agent"
	PUBLIC_RETRY_AFTER_HEADER = "Retry-After"
	PUBLIC_READ_ERROR         = "%s: read response: %w"
	PUBLIC_SIZE_ERROR         = "%s: response exceeds %d bytes"
	PUBLIC_REQUEST_ERROR      = "%s: %w"
	PUBLIC_STATUS_ERROR       = "%s: HTTP %d"
)

type publicStatusError struct {
	source     string
	code       int
	retryAfter string
}

func (e *publicStatusError) Error() string {
	return fmt.Sprintf(PUBLIC_STATUS_ERROR, e.source, e.code)
}

func fetchPublicPosts(ctx context.Context, client *http.Client, source Source, limit int, since time.Time, maxPages int) ([]RawPost, error) {
	switch source.Type {
	case TYPE_TELEGRAM:
		return fetchTelegram(ctx, client, source, limit, since, maxPages)
	case TYPE_REDDIT:
		return fetchReddit(ctx, client, source, limit, since, maxPages)
	default:
		return nil, ErrNotLoadable
	}
}

func readPublicPage(ctx context.Context, client *http.Client, source, endpoint string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, PUBLIC_REQUEST_TIMEOUT)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(PUBLIC_USER_AGENT_HEADER, PUBLIC_READER_AGENT)
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf(PUBLIC_REQUEST_ERROR, source, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, &publicStatusError{source, res.StatusCode, res.Header.Get(PUBLIC_RETRY_AFTER_HEADER)}
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, PUBLIC_PAGE_BYTES+1))
	if err != nil {
		return nil, fmt.Errorf(PUBLIC_READ_ERROR, source, err)
	}
	if len(body) > PUBLIC_PAGE_BYTES {
		return nil, fmt.Errorf(PUBLIC_SIZE_ERROR, source, PUBLIC_PAGE_BYTES)
	}
	return body, nil
}

func waitForPublicPage(ctx context.Context) error {
	timer := time.NewTimer(PUBLIC_PAGE_DELAY)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
