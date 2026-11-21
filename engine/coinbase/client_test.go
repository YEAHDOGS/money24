package coinbase

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// serve points the package at a local server for one test.
func serve(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	old := apiBase
	apiBase = srv.URL
	t.Cleanup(func() {
		apiBase = old
		srv.Close()
	})
}

func TestCandlesComeBackOldestFirst(t *testing.T) {
	var path, query string
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		path, query = r.URL.Path, r.URL.RawQuery
		// Coinbase's order: newest first, each row time, low, high, open, close, volume. One row is short.
		w.Write([]byte(`[[7200,98,103,100,102,5.5],[3600,95,101,96,100,4],[0,1],[0,90,97,91,96,3.25]]`))
	})
	got, err := Candles(context.Background(), http.DefaultClient, "btc-usd", GRANULARITY_HOUR, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/products/BTC-USD/candles" || query != "granularity=3600" {
		t.Errorf("asked %s?%s, want /products/BTC-USD/candles?granularity=3600", path, query)
	}
	want := []Candle{{T: 0, L: 90, H: 97, O: 91, C: 96, V: 3.25}, {T: 3600, L: 95, H: 101, O: 96, C: 100, V: 4}, {T: 7200, L: 98, H: 103, O: 100, C: 102, V: 5.5}}
	if len(got) != len(want) {
		t.Fatalf("%d candles, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candle %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestCandlesRangeAndErrors(t *testing.T) {
	status := http.StatusOK
	var query string
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.WriteHeader(status)
		w.Write([]byte(`[]`))
	})
	ctx, end := context.Background(), time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if _, err := Candles(ctx, http.DefaultClient, "ETH", GRANULARITY_HOUR, end.Add(-CANDLES_MAX*time.Hour), end); err != nil {
		t.Fatal(err)
	}
	if want := "end=2026-11-21T12%3A00%3A00Z&granularity=3600&start=2026-11-09T00%3A00%3A00Z"; query != want {
		t.Errorf("query %s, want %s", query, want)
	}
	if _, err := Candles(ctx, http.DefaultClient, "ETH", GRANULARITY_HOUR, end.Add(-(CANDLES_MAX+1)*time.Hour), end); !errors.Is(err, ErrBadRange) {
		t.Errorf("a range over %d candles: %v, want ErrBadRange", CANDLES_MAX, err)
	}
	if _, err := Candles(ctx, http.DefaultClient, "../etc", GRANULARITY_HOUR, time.Time{}, time.Time{}); !errors.Is(err, ErrBadSymbol) {
		t.Errorf("a path as a symbol: %v, want ErrBadSymbol", err)
	}
	status = http.StatusTooManyRequests
	if _, err := Candles(ctx, http.DefaultClient, "ETH", GRANULARITY_HOUR, time.Time{}, time.Time{}); !errors.Is(err, ErrRateLimited) {
		t.Errorf("HTTP 429: %v, want ErrRateLimited", err)
	}
	status = http.StatusNotFound
	if _, err := Candles(ctx, http.DefaultClient, "NOPE", GRANULARITY_HOUR, time.Time{}, time.Time{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("HTTP 404: %v, want ErrNotFound", err)
	}
}

func TestFetchTakesThePairName(t *testing.T) {
	var path string
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Write([]byte(`{"bid":"60112.29","ask":"60112.3","price":"60112.29","volume":"17225.28"}`))
	})
	tick, err := Fetch(context.Background(), http.DefaultClient, " btc-usd ")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/products/BTC-USD/ticker" || tick.Bid != 60112.29 || tick.Ask != 60112.3 {
		t.Errorf("path %s, ticker %+v", path, tick)
	}
}
