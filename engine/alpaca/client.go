// Package alpaca is Money 24's broker connection: Alpaca's trading API for stocks, ETFs, options and
// crypto. The paper account uses the same API as a live one, so desk trades placed here run through
// the exact code real money would. Keys come from the environment and go only to Alpaca.
// Docs: https://docs.alpaca.markets/reference
package alpaca

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	ENV_KEY_ID  = "ALPACA_KEY_ID"
	ENV_SECRET  = "ALPACA_SECRET_KEY"
	ENV_MODE    = "ALPACA_ENV"
	ENV_FEED    = "ALPACA_DATA_FEED"
	MODE_PAPER  = "paper"
	MODE_LIVE   = "live"
	FEED_IEX    = "iex"
	PAPER_URL   = "https://paper-api.alpaca.markets"
	LIVE_URL    = "https://api.alpaca.markets"
	DATA_URL    = "https://data.alpaca.markets"
	HEADER_KEY  = "APCA-API-KEY-ID"
	HEADER_SEC  = "APCA-API-SECRET-KEY"
	TIMEOUT     = 15 * time.Second
	MAX_BODY    = 4 << 20
	ERROR_CHARS = 300

	PATH_ACCOUNT  = "/v2/account"
	PATH_CONFIG   = "/v2/account/configurations"
	PATH_ORDERS   = "/v2/orders"
	PATH_BY_ID    = "/v2/orders:by_client_order_id"
	PATH_POSITION = "/v2/positions"
	PATH_CRYPTO_Q = "/v1beta3/crypto/us/latest/quotes"

	SIDE_BUY     = "buy"
	SIDE_SELL    = "sell"
	TYPE_LIMIT   = "limit"
	TYPE_MARKET  = "market"
	TIF_DAY      = "day"
	TIF_GTC      = "gtc"
	CASH_ONLY    = "1" // max_margin_multiplier: no borrowing, the account behaves as cash
	CRYPTO_QUOTE = "/USD"
	PAIR_SUFFIX  = "-USD"
	STATUS_DUPE  = http.StatusUnprocessableEntity // Alpaca answers 422 for a client_order_id it already has
	DUPE_TEXT    = "client_order_id must be unique"
)

var (
	ErrNoKeys = errors.New("alpaca: set ALPACA_KEY_ID and ALPACA_SECRET_KEY in .env")
	ErrLive   = errors.New("alpaca: ALPACA_ENV=live is refused: no desk has cleared the go-live bar")
	ErrDupe   = errors.New("alpaca: that order was already placed")
)

// Client talks to one Alpaca account.
type Client struct {
	KeyID, Secret string
	Mode          string // paper or live
	Feed          string
	HTTP          *http.Client
	base          string
}

// FromEnv builds the client from .env, or returns nil when no key is set. Live keys are refused.
func FromEnv() (*Client, error) {
	key, secret := os.Getenv(ENV_KEY_ID), os.Getenv(ENV_SECRET)
	if key == "" && secret == "" {
		return nil, nil
	}
	if key == "" || secret == "" {
		return nil, ErrNoKeys
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(ENV_MODE)))
	if mode == "" {
		mode = MODE_PAPER
	}
	if mode != MODE_PAPER {
		return nil, ErrLive
	}
	feed := os.Getenv(ENV_FEED)
	if feed == "" {
		feed = FEED_IEX
	}
	return &Client{KeyID: key, Secret: secret, Mode: mode, Feed: feed, HTTP: &http.Client{Timeout: TIMEOUT}, base: PAPER_URL}, nil
}

// do sends one request and decodes the answer into out (when out is not nil).
func (c *Client) do(ctx context.Context, method, base, path string, query url.Values, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	req.Header.Set(HEADER_KEY, c.KeyID)
	req.Header.Set(HEADER_SEC, c.Secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, MAX_BODY))
	if res.StatusCode == STATUS_DUPE && strings.Contains(string(raw), DUPE_TEXT) {
		return ErrDupe
	}
	if res.StatusCode >= http.StatusBadRequest {
		text := string(raw)
		if len(text) > ERROR_CHARS {
			text = text[:ERROR_CHARS]
		}
		return fmt.Errorf("alpaca: %s %s: HTTP %d: %s", method, path, res.StatusCode, text)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Account is the part of Alpaca's account the desk reads. Money fields arrive as strings.
type Account struct {
	Status          string  `json:"status"`
	Currency        string  `json:"currency"`
	Cash            float64 `json:"cash,string"`
	Equity          float64 `json:"equity,string"`
	BuyingPower     float64 `json:"buying_power,string"`
	NonMarginBP     float64 `json:"non_marginable_buying_power,string"`
	Multiplier      string  `json:"multiplier"`
	CryptoStatus    string  `json:"crypto_status"`
	OptionsApproved int     `json:"options_approved_level"`
	OptionsLevel    int     `json:"options_trading_level"`
	PatternDay      bool    `json:"pattern_day_trader"`
	TradingBlocked  bool    `json:"trading_blocked"`
}

// Account reads the account.
func (c *Client) Account(ctx context.Context) (Account, error) {
	var a Account
	return a, c.do(ctx, http.MethodGet, c.base, PATH_ACCOUNT, nil, nil, &a)
}

// Config is the account settings the desk sets.
type Config struct {
	MaxMargin  string `json:"max_margin_multiplier"`
	NoShorting bool   `json:"no_shorting"`
}

// CashOnly makes the account behave as a cash account: no borrowing, no short selling.
func (c *Client) CashOnly(ctx context.Context) (Config, error) {
	var out Config
	return out, c.do(ctx, http.MethodPatch, c.base, PATH_CONFIG, nil, Config{MaxMargin: CASH_ONLY, NoShorting: true}, &out)
}

// Order is one order as it is sent and as Alpaca reports it.
type Order struct {
	ID            string `json:"id,omitempty"`
	ClientOrderID string `json:"client_order_id"`
	Symbol        string `json:"symbol"`
	Qty           string `json:"qty,omitempty"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	TimeInForce   string `json:"time_in_force"`
	LimitPrice    string `json:"limit_price,omitempty"`
	Status        string `json:"status,omitempty"`
	FilledQty     string `json:"filled_qty,omitempty"`
	FilledAvg     string `json:"filled_avg_price,omitempty"`
	SubmittedAt   string `json:"submitted_at,omitempty"`
	FilledAt      string `json:"filled_at,omitempty"`
}

// Place sends an order. The client order id makes it safe to repeat: a second try is ErrDupe.
func (c *Client) Place(ctx context.Context, o Order) (Order, error) {
	var out Order
	return out, c.do(ctx, http.MethodPost, c.base, PATH_ORDERS, nil, o, &out)
}

// ByClientID reads an order by the id the desk gave it.
func (c *Client) ByClientID(ctx context.Context, id string) (Order, error) {
	var out Order
	return out, c.do(ctx, http.MethodGet, c.base, PATH_BY_ID, url.Values{"client_order_id": {id}}, nil, &out)
}

// Cancel cancels an open order by Alpaca's id.
func (c *Client) Cancel(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, c.base, PATH_ORDERS+"/"+url.PathEscape(id), nil, nil, nil)
}

// Orders lists recent orders, newest first.
func (c *Client) Orders(ctx context.Context, status string, limit int) ([]Order, error) {
	var out []Order
	return out, c.do(ctx, http.MethodGet, c.base, PATH_ORDERS, url.Values{"status": {status}, "limit": {fmt.Sprint(limit)}, "direction": {"desc"}}, nil, &out)
}

// Position is one holding as Alpaca reports it.
type Position struct {
	Symbol        string  `json:"symbol"`
	AssetClass    string  `json:"asset_class"`
	Qty           float64 `json:"qty,string"`
	AvgEntry      float64 `json:"avg_entry_price,string"`
	MarketValue   float64 `json:"market_value,string"`
	UnrealizedPnL float64 `json:"unrealized_pl,string"`
}

// Positions lists every holding.
func (c *Client) Positions(ctx context.Context) ([]Position, error) {
	var out []Position
	return out, c.do(ctx, http.MethodGet, c.base, PATH_POSITION, nil, nil, &out)
}

// CryptoQuote is the best bid and ask for a coin pair.
type CryptoQuote struct {
	Bid float64 `json:"bp"`
	Ask float64 `json:"ap"`
}

// LatestCrypto reads the latest quote for a pair such as BTC/USD.
func (c *Client) LatestCrypto(ctx context.Context, pair string) (CryptoQuote, error) {
	var out struct {
		Quotes map[string]CryptoQuote `json:"quotes"`
	}
	if err := c.do(ctx, http.MethodGet, DATA_URL, PATH_CRYPTO_Q, url.Values{"symbols": {pair}}, nil, &out); err != nil {
		return CryptoQuote{}, err
	}
	q, ok := out.Quotes[pair]
	if !ok {
		return CryptoQuote{}, fmt.Errorf("alpaca: no quote for %s", pair)
	}
	return q, nil
}

// CryptoPair turns a desk symbol (BTC-USD, or BTC) into Alpaca's (BTC/USD).
func CryptoPair(symbol string) string {
	return strings.ToUpper(strings.TrimSuffix(symbol, PAIR_SUFFIX)) + CRYPTO_QUOTE
}
