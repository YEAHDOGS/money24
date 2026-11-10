// Package coinbase reads top-of-book crypto prices from Coinbase Exchange's public market-data
// API. It needs no key and places no orders. Docs: https://docs.cdp.coinbase.com/exchange/
package coinbase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const (
	TICKER_URL         = "https://api.exchange.coinbase.com/products/%s-%s/ticker"
	QUOTE_CURRENCY     = "USD"
	USER_AGENT         = "money24-m24d/1.0" // the API refuses requests without one
	HEADER_USER_AGENT  = "User-Agent"
	MAX_RESPONSE_BYTES = 1 << 16
)

var (
	ErrNotFound  = errors.New("coinbase: no USD market for symbol")
	ErrBadSymbol = errors.New("coinbase: symbol must be 1 to 10 letters or digits")
	// Symbols come from parsed posts and go into a URL path, so anything else is refused.
	SYMBOL = regexp.MustCompile(`^[A-Z0-9]{1,10}$`)
)

// Ticker is the current market for one coin against the US dollar.
type Ticker struct {
	Bid    float64 `json:"bid,string"`
	Ask    float64 `json:"ask,string"`
	Price  float64 `json:"price,string"`  // last trade
	Volume float64 `json:"volume,string"` // coins traded in the last 24 hours
}

// Fetch returns the ticker for symbol (BTC, eth, PEPE) priced in US dollars.
func Fetch(ctx context.Context, client *http.Client, symbol string) (Ticker, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if !SYMBOL.MatchString(symbol) {
		return Ticker{}, ErrBadSymbol
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(TICKER_URL, symbol, QUOTE_CURRENCY), nil)
	if err != nil {
		return Ticker{}, err
	}
	req.Header.Set(HEADER_USER_AGENT, USER_AGENT)
	res, err := client.Do(req)
	if err != nil {
		return Ticker{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return Ticker{}, fmt.Errorf("%w: %s", ErrNotFound, symbol)
	}
	if res.StatusCode != http.StatusOK {
		return Ticker{}, fmt.Errorf("coinbase: HTTP %d for %s", res.StatusCode, symbol)
	}
	var t Ticker
	if err := json.NewDecoder(io.LimitReader(res.Body, MAX_RESPONSE_BYTES)).Decode(&t); err != nil {
		return Ticker{}, fmt.Errorf("coinbase: %s: %w", symbol, err)
	}
	return t, nil
}
