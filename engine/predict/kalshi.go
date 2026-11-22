// Package predict is the prediction-market desk: a paper book that trades Kalshi event contracts at
// Kalshi's real, live prices. It reads Kalshi's public market data (no key needed), fills paper
// orders against the real order book with Kalshi's real fee, and settles each contract at the real
// result: $1 a contract to the side that won, nothing to the side that lost. No order ever reaches
// Kalshi from here; this is how a strategy earns a record before any money or demo account is used.
package predict

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	KALSHI_URL        = "https://api.elections.kalshi.com/trade-api/v2" // public market data for every Kalshi market
	KALSHI_USER_AGENT = "Money24/0.1"
	KALSHI_PAGE       = 1000 // markets per page, Kalshi's maximum
	KALSHI_PAGES_MAX  = 20   // a scan never reads more pages than this
	KALSHI_TICKERS    = 50   // markets looked up by ticker in one request
	KALSHI_BODY_MAX   = 32 << 20
	REQUEST_GAP       = 120 * time.Millisecond // Kalshi's basic tier allows 20 reads a second; stay well under
	STATUS_OPEN_QUERY = "open"
	MVE_EXCLUDE       = "exclude" // multivariate combos (parlays) are left out: thin, and priced as bundles
	MARKET_BINARY     = "binary"
	RESULT_YES        = "yes"
	RESULT_NO         = "no"
	SIDE_YES          = "yes"
	SIDE_NO           = "no"
	DOLLAR            = 1.0

	// Keys for Kalshi's authenticated API (demo orders, later). Market data needs none of them.
	ENV_KALSHI_KEY_ID = "KALSHI_KEY_ID"
	ENV_KALSHI_KEY    = "KALSHI_PRIVATE_KEY_PATH"
	ENV_KALSHI_ENV    = "KALSHI_ENV"
	KALSHI_ENV_DEMO   = "demo"
)

// FINISHED are the market states in which the result is known.
var FINISHED = map[string]bool{"determined": true, "finalized": true, "settled": true}

// Market is one Kalshi market as the desk uses it. Prices are dollars a contract, 0 to 1.
type Market struct {
	Ticker    string
	Event     string
	Title     string
	YesTitle  string
	Type      string
	Status    string
	Result    string
	Value     float64 // what one YES contract settled at: 1 when yes won, 0 when no won
	HasValue  bool
	CloseAt   time.Time
	YesBid    float64
	YesAsk    float64
	NoBid     float64
	NoAsk     float64
	Volume24h float64
}

// Finished says whether the market's result is known.
func (m Market) Finished() bool {
	return FINISHED[m.Status] && (m.Result == RESULT_YES || m.Result == RESULT_NO)
}

// Ask is what one contract of a side costs at the top of the book, and Bid what it sells for.
func (m Market) Ask(side string) float64 {
	if side == SIDE_YES {
		return m.YesAsk
	}
	return m.NoAsk
}

func (m Market) Bid(side string) float64 {
	if side == SIDE_YES {
		return m.YesBid
	}
	return m.NoBid
}

// Payout is what one contract of a side pays at settlement.
func (m Market) Payout(side string) float64 {
	value := m.Value
	if !m.HasValue {
		value = 0
		if m.Result == RESULT_YES {
			value = DOLLAR
		}
	}
	if side == SIDE_YES {
		return value
	}
	return DOLLAR - value
}

// Level is one price in an order book: what one contract costs there, and how many are offered.
type Level struct {
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
}

// Kalshi reads Kalshi's public market data.
type Kalshi struct {
	HTTP *http.Client
	Base string
}

// KeysSet says whether Kalshi API keys are in the environment (.env), for orders on Kalshi later.
func KeysSet() bool {
	return os.Getenv(ENV_KALSHI_KEY_ID) != "" && os.Getenv(ENV_KALSHI_KEY) != ""
}

type wireMarket struct {
	Ticker      string `json:"ticker"`
	EventTicker string `json:"event_ticker"`
	Title       string `json:"title"`
	YesSubTitle string `json:"yes_sub_title"`
	MarketType  string `json:"market_type"`
	Status      string `json:"status"`
	Result      string `json:"result"`
	Settlement  string `json:"settlement_value_dollars"`
	CloseTime   string `json:"close_time"`
	YesBid      string `json:"yes_bid_dollars"`
	YesAsk      string `json:"yes_ask_dollars"`
	NoBid       string `json:"no_bid_dollars"`
	NoAsk       string `json:"no_ask_dollars"`
	Volume24h   string `json:"volume_24h_fp"`
}

func number(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return n
}

func (w wireMarket) market() Market {
	closeAt, _ := time.Parse(time.RFC3339, w.CloseTime)
	m := Market{Ticker: w.Ticker, Event: w.EventTicker, Title: w.Title, YesTitle: w.YesSubTitle, Type: w.MarketType,
		Status: w.Status, Result: w.Result, CloseAt: closeAt, YesBid: number(w.YesBid), YesAsk: number(w.YesAsk),
		NoBid: number(w.NoBid), NoAsk: number(w.NoAsk), Volume24h: number(w.Volume24h)}
	if w.Settlement != "" {
		m.Value, m.HasValue = number(w.Settlement), true
	}
	return m
}

func (k *Kalshi) get(ctx context.Context, path string, query url.Values, out any) error {
	base := k.Base
	if base == "" {
		base = KALSHI_URL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", KALSHI_USER_AGENT)
	req.Header.Set("Accept", "application/json")
	res, err := k.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, KALSHI_BODY_MAX))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("kalshi %s: %s", path, res.Status)
	}
	return json.Unmarshal(body, out)
}

// pause waits between requests, and gives up when ctx ends.
func pause(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(REQUEST_GAP):
		return nil
	}
}

// Closing returns the open markets whose trading ends between from and to, combos left out.
func (k *Kalshi) Closing(ctx context.Context, from, to time.Time) ([]Market, error) {
	query := url.Values{"limit": {strconv.Itoa(KALSHI_PAGE)}, "status": {STATUS_OPEN_QUERY}, "mve_filter": {MVE_EXCLUDE},
		"min_close_ts": {strconv.FormatInt(from.Unix(), 10)}, "max_close_ts": {strconv.FormatInt(to.Unix(), 10)}}
	out := []Market{}
	for page := 0; page < KALSHI_PAGES_MAX; page++ {
		var doc struct {
			Cursor  string       `json:"cursor"`
			Markets []wireMarket `json:"markets"`
		}
		if err := k.get(ctx, "/markets", query, &doc); err != nil {
			return out, err
		}
		for _, w := range doc.Markets {
			out = append(out, w.market())
		}
		if doc.Cursor == "" || len(doc.Markets) < KALSHI_PAGE {
			return out, nil
		}
		query.Set("cursor", doc.Cursor)
		if err := pause(ctx); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Lookup returns the named markets, open or settled, by ticker.
func (k *Kalshi) Lookup(ctx context.Context, tickers []string) (map[string]Market, error) {
	out := map[string]Market{}
	for start := 0; start < len(tickers); start += KALSHI_TICKERS {
		batch := tickers[start:min(len(tickers), start+KALSHI_TICKERS)]
		var doc struct {
			Markets []wireMarket `json:"markets"`
		}
		query := url.Values{"tickers": {strings.Join(batch, ",")}, "limit": {strconv.Itoa(KALSHI_TICKERS)}}
		if err := k.get(ctx, "/markets", query, &doc); err != nil {
			return out, err
		}
		for _, w := range doc.Markets {
			out[w.Ticker] = w.market()
		}
		if err := pause(ctx); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Asks returns what one side's contracts are offered at, cheapest first. Kalshi's book lists only
// bids: a bid for NO at p is an offer to sell YES at 1 - p, and the other way round.
func (k *Kalshi) Asks(ctx context.Context, ticker, side string) ([]Level, error) {
	var doc struct {
		Book struct {
			Yes [][]string `json:"yes_dollars"`
			No  [][]string `json:"no_dollars"`
		} `json:"orderbook_fp"`
	}
	if err := k.get(ctx, "/markets/"+url.PathEscape(ticker)+"/orderbook", url.Values{}, &doc); err != nil {
		return nil, err
	}
	opposite := doc.Book.No
	if side == SIDE_NO {
		opposite = doc.Book.Yes
	}
	out := []Level{}
	for _, row := range opposite {
		if len(row) < 2 {
			continue
		}
		bid, qty := number(row[0]), number(row[1])
		if bid <= 0 || bid >= DOLLAR || qty <= 0 {
			continue
		}
		out = append(out, Level{Price: round(DOLLAR-bid, PRICE_DIGITS), Qty: qty})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Price < out[j].Price })
	return out, nil
}
