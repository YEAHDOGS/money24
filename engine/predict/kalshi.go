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
	"math"
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
	REQUEST_GAP       = 200 * time.Millisecond // Kalshi's basic tier allows 20 reads a second; stay well under
	KALSHI_TRIES      = 4
	KALSHI_BACKOFF    = 2 * time.Second
	STATUS_OPEN_QUERY = "open"
	MVE_EXCLUDE       = "exclude" // multivariate combos (parlays) are left out: thin, and priced as bundles
	MARKET_BINARY     = "binary"
	STATUS_FINALIZED  = "finalized"
	STATUS_SETTLED    = "settled"
	RESULT_YES        = "yes"
	RESULT_NO         = "no"
	RESULT_SCALAR     = "scalar"
	SIDE_YES          = "yes"
	SIDE_NO           = "no"
	DOLLAR            = 1.0
	EVENT_SEPARATOR   = "-"

	// Keys for Kalshi's authenticated API (demo orders, later). Market data needs none of them.
	ENV_KALSHI_KEY_ID = "KALSHI_KEY_ID"
	ENV_KALSHI_KEY    = "KALSHI_PRIVATE_KEY_PATH"
	ENV_KALSHI_ENV    = "KALSHI_ENV"
	KALSHI_ENV_DEMO   = "demo"
)

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
	Strike    string  // greater, less or between
	Floor     float64 // greater: YES above it; between: the lowest value YES covers
	Cap       float64 // less: YES below it; between: the highest value YES covers
}

// Series is the market's series ticker: its event ticker up to the date.
func (m Market) Series() string {
	series, _, _ := strings.Cut(m.Event, EVENT_SEPARATOR)
	return series
}

// EventDate is the day the market is about, from its event ticker (KXHIGHNY-26NOV21).
func (m Market) EventDate() (time.Time, error) {
	_, date, _ := strings.Cut(m.Event, EVENT_SEPARATOR)
	if len(date) != len(EVENT_DATE_LAYOUT) {
		return time.Time{}, fmt.Errorf("no date in event %q", m.Event)
	}
	// 26NOV21 -> 26Nov21, the way Go spells a month
	return time.Parse(EVENT_DATE_LAYOUT, date[:3]+strings.ToLower(date[3:5])+date[5:])
}

// ContinuousRange is the values that settle YES for a quantity that is not rounded, like a price:
// greater is above the floor, less is below the cap, between runs from the floor to the cap.
func (m Market) ContinuousRange() (lo, hi float64, ok bool) {
	switch m.Strike {
	case STRIKE_GREATER:
		return m.Floor, math.Inf(1), true
	case STRIKE_LESS:
		return math.Inf(-1), m.Cap, true
	case STRIKE_BETWEEN:
		return m.Floor, m.Cap, m.Cap >= m.Floor
	}
	return 0, 0, false
}

// Range is the values that settle YES, as a half-open range of a quantity reported in whole units.
func (m Market) Range() (lo, hi float64, ok bool) {
	switch m.Strike {
	case STRIKE_GREATER:
		return m.Floor + HALF_DEGREE, math.Inf(1), true
	case STRIKE_LESS:
		return math.Inf(-1), m.Cap - HALF_DEGREE, true
	case STRIKE_BETWEEN:
		return m.Floor - HALF_DEGREE, m.Cap + HALF_DEGREE, m.Cap >= m.Floor
	}
	return 0, 0, false
}

// Finished says whether the market has a final, payable result.
func (m Market) Finished() bool {
	if m.Status != STATUS_FINALIZED && m.Status != STATUS_SETTLED {
		return false
	}
	return m.Result == RESULT_YES || m.Result == RESULT_NO || (m.Result == RESULT_SCALAR && m.HasValue)
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
	Ticker      string   `json:"ticker"`
	EventTicker string   `json:"event_ticker"`
	Title       string   `json:"title"`
	YesSubTitle string   `json:"yes_sub_title"`
	MarketType  string   `json:"market_type"`
	Status      string   `json:"status"`
	Result      string   `json:"result"`
	Settlement  string   `json:"settlement_value_dollars"`
	CloseTime   string   `json:"close_time"`
	YesBid      string   `json:"yes_bid_dollars"`
	YesAsk      string   `json:"yes_ask_dollars"`
	NoBid       string   `json:"no_bid_dollars"`
	NoAsk       string   `json:"no_ask_dollars"`
	Volume24h   string   `json:"volume_24h_fp"`
	StrikeType  string   `json:"strike_type"`
	FloorStrike *float64 `json:"floor_strike"`
	CapStrike   *float64 `json:"cap_strike"`
}

func number(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return n
}

func (w wireMarket) market() Market {
	closeAt, _ := time.Parse(time.RFC3339, w.CloseTime)
	m := Market{Ticker: w.Ticker, Event: w.EventTicker, Title: w.Title, YesTitle: w.YesSubTitle, Type: w.MarketType,
		Status: w.Status, Result: w.Result, CloseAt: closeAt, YesBid: number(w.YesBid), YesAsk: number(w.YesAsk),
		NoBid: number(w.NoBid), NoAsk: number(w.NoAsk), Volume24h: number(w.Volume24h),
		Strike: w.StrikeType}
	if w.FloorStrike != nil {
		m.Floor = *w.FloorStrike
	}
	if w.CapStrike != nil {
		m.Cap = *w.CapStrike
	}
	switch {
	case (w.StrikeType == STRIKE_GREATER || w.StrikeType == STRIKE_GREATER_OR_EQUAL) && w.FloorStrike == nil,
		w.StrikeType == STRIKE_LESS && w.CapStrike == nil,
		w.StrikeType == STRIKE_BETWEEN && (w.FloorStrike == nil || w.CapStrike == nil):
		m.Strike = ""
	}
	if value, err := strconv.ParseFloat(strings.TrimSpace(w.Settlement), 64); err == nil && value >= 0 && value <= DOLLAR {
		m.Value, m.HasValue = value, true
	}
	return m
}

// get reads one Kalshi endpoint. Told to slow down (429), it waits and tries again, a little longer each time.
func (k *Kalshi) get(ctx context.Context, path string, query url.Values, out any) error {
	base := k.Base
	if base == "" {
		base = KALSHI_URL
	}
	for try := 1; ; try++ {
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
		body, err := io.ReadAll(io.LimitReader(res.Body, KALSHI_BODY_MAX))
		res.Body.Close()
		if err != nil {
			return err
		}
		if res.StatusCode == http.StatusTooManyRequests && try < KALSHI_TRIES {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(try) * KALSHI_BACKOFF):
			}
			continue
		}
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("kalshi %s: %s", path, res.Status)
		}
		return json.Unmarshal(body, out)
	}
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
	return k.markets(ctx, url.Values{"mve_filter": {MVE_EXCLUDE},
		"min_close_ts": {strconv.FormatInt(from.Unix(), 10)}, "max_close_ts": {strconv.FormatInt(to.Unix(), 10)}})
}

// InSeries returns the open markets of one series, only those closing by to when to is set.
func (k *Kalshi) InSeries(ctx context.Context, series string, to time.Time) ([]Market, error) {
	query := url.Values{"series_ticker": {series}}
	if !to.IsZero() {
		query.Set("max_close_ts", strconv.FormatInt(to.Unix(), 10))
	}
	return k.markets(ctx, query)
}

// markets pages through the open markets a query matches.
func (k *Kalshi) markets(ctx context.Context, query url.Values) ([]Market, error) {
	query.Set("limit", strconv.Itoa(KALSHI_PAGE))
	query.Set("status", STATUS_OPEN_QUERY)
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
