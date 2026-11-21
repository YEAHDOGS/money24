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
	API_BASE           = "https://api.exchange.coinbase.com"
	TICKER_PATH        = "/products/%s-%s/ticker"
	QUOTE_CURRENCY     = "USD"
	PAIR_SUFFIX        = "-" + QUOTE_CURRENCY // BTC-USD names the same market as BTC
	USER_AGENT         = "money24-m24d/1.0"   // the API refuses requests without one
	HEADER_USER_AGENT  = "User-Agent"
	MAX_RESPONSE_BYTES = 1 << 16
	// TAKER_FEE_PCT is the fee assumed on each side of a coin trade, as a percent of the trade.
	// It is Coinbase Advanced's long-standing entry-tier taker rate. It is an assumption, not a
	// reading of the account: set it to what the venue you would really trade on charges. Desk
	// prices carry it (buys cost this much more, sells pay this much less), so results do not flatter.
	TAKER_FEE_PCT = 0.6
)

var (
	ErrNotFound  = errors.New("coinbase: no USD market for symbol")
	ErrBadSymbol = errors.New("coinbase: symbol must be 1 to 10 letters or digits")
	// Symbols come from parsed posts and go into a URL path, so anything else is refused.
	SYMBOL = regexp.MustCompile(`^[A-Z0-9]{1,10}$`)
	// apiBase is API_BASE everywhere but in this package's tests, which point it at a local server.
	apiBase = API_BASE
)

// Ticker is the current market for one coin against the US dollar.
type Ticker struct {
	Bid    float64 `json:"bid,string"`
	Ask    float64 `json:"ask,string"`
	Price  float64 `json:"price,string"`  // last trade
	Volume float64 `json:"volume,string"` // coins traded in the last 24 hours
}

// Base is the coin's own ticker: "btc-usd" and "BTC" both give BTC. Anything that is not a plain
// ticker is refused, because it goes into a URL path.
func Base(symbol string) (string, error) {
	symbol = strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(symbol)), PAIR_SUFFIX)
	if !SYMBOL.MatchString(symbol) {
		return "", ErrBadSymbol
	}
	return symbol, nil
}

// Fetch returns the ticker for symbol (BTC, eth, PEPE, BTC-USD) priced in US dollars.
func Fetch(ctx context.Context, client *http.Client, symbol string) (Ticker, error) {
	symbol, err := Base(symbol)
	if err != nil {
		return Ticker{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+fmt.Sprintf(TICKER_PATH, symbol, QUOTE_CURRENCY), nil)
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
