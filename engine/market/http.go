package market

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	ROUTE_PREFIX = "GET /api/market/"
	// How long an answer is reused. Every open page asks on a timer, so this is what keeps the
	// request count to the provider flat no matter how many windows are open.
	TTL_LIVE    = 10 * time.Second // candles and quotes
	TTL_SEARCH  = 2 * time.Minute
	TTL_RELATED = 10 * time.Minute
	CACHE_MAX   = 400 // answers kept; the whole cache is dropped when it fills
	SYMBOL_SEP  = ","
)

type cached struct {
	at   time.Time
	body []byte
}

type cache struct {
	mu    sync.Mutex
	items map[string]cached
}

// serve answers from the cache when it can, otherwise loads, stores and answers.
func (c *Client) serve(w http.ResponseWriter, r *http.Request, ttl time.Duration, load func(context.Context) (any, error)) {
	key := r.URL.Path + "?" + r.URL.RawQuery
	c.cache.mu.Lock()
	hit, ok := c.cache.items[key]
	c.cache.mu.Unlock()
	if !ok || time.Since(hit.at) > ttl {
		v, err := load(r.Context())
		switch {
		case errors.Is(err, ErrBadInput):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		case errors.Is(err, ErrNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		case err != nil:
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		body, err := json.Marshal(v)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hit = cached{at: time.Now(), body: body}
		c.cache.mu.Lock()
		if len(c.cache.items) >= CACHE_MAX {
			c.cache.items = map[string]cached{}
		}
		c.cache.items[key] = hit
		c.cache.mu.Unlock()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(hit.body)
}

// Handler serves the Markets page. Mount it at ROUTE_PREFIX.
//
//	GET /api/market/chart?symbol=&range=&interval=   candles
//	GET /api/market/summary?symbol=                  quote, statistics, profile, quarters
//	GET /api/market/search?q=                        matching tickers and news
//	GET /api/market/quotes?symbols=A,B               short quotes for a list
//	GET /api/market/related?symbol=                  short quotes for related tickers
func Handler(c *Client) http.Handler {
	symbol := func(r *http.Request) string { return strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol"))) }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/market/chart", func(w http.ResponseWriter, r *http.Request) {
		c.serve(w, r, TTL_LIVE, func(ctx context.Context) (any, error) {
			return c.Chart(ctx, symbol(r), r.URL.Query().Get("range"), r.URL.Query().Get("interval"))
		})
	})
	mux.HandleFunc("GET /api/market/summary", func(w http.ResponseWriter, r *http.Request) {
		c.serve(w, r, TTL_LIVE, func(ctx context.Context) (any, error) { return c.Summary(ctx, symbol(r)) })
	})
	mux.HandleFunc("GET /api/market/search", func(w http.ResponseWriter, r *http.Request) {
		c.serve(w, r, TTL_SEARCH, func(ctx context.Context) (any, error) { return c.Search(ctx, r.URL.Query().Get("q")) })
	})
	mux.HandleFunc("GET /api/market/quotes", func(w http.ResponseWriter, r *http.Request) {
		c.serve(w, r, TTL_LIVE, func(ctx context.Context) (any, error) {
			return c.Ticks(ctx, strings.Split(strings.ToUpper(r.URL.Query().Get("symbols")), SYMBOL_SEP))
		})
	})
	mux.HandleFunc("GET /api/market/related", func(w http.ResponseWriter, r *http.Request) {
		c.serve(w, r, TTL_RELATED, func(ctx context.Context) (any, error) { return c.Related(ctx, symbol(r)) })
	})
	return mux
}
