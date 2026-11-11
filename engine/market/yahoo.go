// Package market reads public market data for the Markets page: candles, a quote summary, search,
// news and related tickers. It only reads. Nothing here can place an order.
//
// The one provider so far is Yahoo Finance's web API. It is unofficial: it needs no key, covers
// stocks, ETFs, indexes, futures, currencies and listed coins, and can change or throttle without
// notice. Quotes for US stocks are delayed by up to 15 minutes outside Nasdaq real-time hours.
package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	YAHOO_API     = "https://query1.finance.yahoo.com"
	CHART_PATH    = "/v8/finance/chart/%s?range=%s&interval=%s&includePrePost=%t"
	HISTORY_PATH  = "/v8/finance/chart/%s?period1=%d&period2=%d&interval=%s"
	SUMMARY_PATH  = "/v10/finance/quoteSummary/%s?modules=price,summaryDetail,earnings,assetProfile"
	SEARCH_PATH   = "/v1/finance/search?q=%s&quotesCount=%d&newsCount=%d"
	QUOTES_PATH   = "/v7/finance/quote?symbols=%s"
	RELATED_PATH  = "/v6/finance/recommendationsbysymbol/%s"
	CRUMB_PATH    = "/v1/test/getcrumb"
	COOKIE_URL    = "https://fc.yahoo.com" // answers 404 but sets the session cookie the crumb needs
	CRUMB_PARAM   = "&crumb="
	HTTPS_PREFIX  = "https://"
	FETCH_TIMEOUT = 8 * time.Second
	MAX_BODY      = 8 << 20
	PERCENT       = 100
	// Yahoo turns away clients that do not look like a browser.
	BROWSER_AGENT   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"
	STATE_PRE       = "PRE"
	STATE_REGULAR   = "REGULAR"
	SESSION_PRE     = "pre"
	SESSION_POST    = "post"
	SEARCH_QUOTES   = 8
	SEARCH_NEWS     = 8
	MAX_QUOTES      = 30
	MAX_QUERY_CHARS = 60
	HISTORY_5M      = 55 * 24 * time.Hour  // Yahoo keeps 5-minute candles for 60 days
	HISTORY_1H      = 700 * 24 * time.Hour // and hourly candles for 730
	THUMB_MIN_WIDTH = 300                  // the smallest thumbnail that still looks sharp on the news card
)

var (
	ErrNotFound = errors.New("symbol not found")
	ErrBadInput = errors.New("bad symbol, range or interval")
	SYMBOL      = regexp.MustCompile(`^[A-Z0-9^][A-Z0-9.=^-]{0,19}$`)
	// Which candle sizes Yahoo serves for each range. The first is the default.
	INTERVALS = map[string][]string{
		"1d":  {"1m", "2m", "5m", "15m"},
		"5d":  {"5m", "1m", "15m", "30m", "1h"},
		"1mo": {"30m", "5m", "15m", "1h", "1d"},
		"6mo": {"1d", "1h", "1wk"},
		"ytd": {"1d", "1h", "1wk"},
		"1y":  {"1d", "1h", "1wk"},
		"5y":  {"1wk", "1d", "1mo"},
		"max": {"1mo", "1d", "1wk"},
	}
	// Ranges short enough to be worth showing the pre-market and after-hours sessions.
	EXTENDED_RANGES = map[string]bool{"1d": true, "5d": true}
)

// Candle is one bar. T is the bar's opening time in Unix seconds.
type Candle struct {
	T int64   `json:"t"`
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`
	V float64 `json:"v"`
}

// Session is one trading period of the current day, in Unix seconds.
type Session struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type Chart struct {
	Symbol   string  `json:"symbol"`
	Range    string  `json:"range"`
	Interval string  `json:"interval"`
	Timezone string  `json:"timezone"` // IANA name of the exchange's zone
	Price    float64 `json:"price"`
	// The close just before the first candle: the baseline a range's change is measured from.
	PreviousClose float64  `json:"previous_close"`
	Regular       Session  `json:"regular"`
	Candles       []Candle `json:"candles"`
}

// Quarter is one reported quarter: what was earned against what analysts expected.
type Quarter struct {
	Label       string  `json:"label"` // "2Q2026"
	Revenue     float64 `json:"revenue"`
	Earnings    float64 `json:"earnings"`
	EPS         float64 `json:"eps"`
	EPSEstimate float64 `json:"eps_estimate"`
}

type Summary struct {
	Symbol         string  `json:"symbol"`
	Name           string  `json:"name"`
	Exchange       string  `json:"exchange"`
	Currency       string  `json:"currency"`
	CurrencySymbol string  `json:"currency_symbol"`
	Type           string  `json:"type"`
	MarketState    string  `json:"market_state"`
	DelayMinutes   int     `json:"delay_minutes"`
	Price          float64 `json:"price"`
	Change         float64 `json:"change"`
	ChangePct      float64 `json:"change_pct"`
	MarketTime     int64   `json:"market_time"`
	PreviousClose  float64 `json:"previous_close"`
	Open           float64 `json:"open"`
	DayHigh        float64 `json:"day_high"`
	DayLow         float64 `json:"day_low"`
	Volume         float64 `json:"volume"`
	AvgVolume      float64 `json:"avg_volume"`
	// The pre-market or after-hours print, when there is one. ExtSession is "pre" or "post".
	ExtSession   string    `json:"ext_session,omitempty"`
	ExtPrice     float64   `json:"ext_price,omitempty"`
	ExtChange    float64   `json:"ext_change,omitempty"`
	ExtChangePct float64   `json:"ext_change_pct,omitempty"`
	ExtTime      int64     `json:"ext_time,omitempty"`
	MarketCap    float64   `json:"market_cap"`
	PE           float64   `json:"pe"`
	DividendPct  float64   `json:"dividend_pct"`
	DividendRate float64   `json:"dividend_rate"` // per share, per year
	High52       float64   `json:"high_52"`
	Low52        float64   `json:"low_52"`
	About        string    `json:"about,omitempty"`
	Website      string    `json:"website,omitempty"`
	Sector       string    `json:"sector,omitempty"`
	Industry     string    `json:"industry,omitempty"`
	Quarters     []Quarter `json:"quarters"`
	NextEarnings int64     `json:"next_earnings,omitempty"`
}

type Match struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
	Type     string `json:"type"`
}

type Story struct {
	Title     string `json:"title"`
	Publisher string `json:"publisher"`
	URL       string `json:"url"`
	At        int64  `json:"at"`
	Thumb     string `json:"thumb,omitempty"`
}

type SearchResult struct {
	Matches []Match `json:"matches"`
	News    []Story `json:"news"`
}

// Tick is the short quote shown in a list of tickers.
type Tick struct {
	Symbol    string  `json:"symbol"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	ChangePct float64 `json:"change_pct"`
	Currency  string  `json:"currency"`
}

// raw is Yahoo's wrapped number: {"raw": 1.5, "fmt": "1.50"}. A missing value arrives as {}.
type raw struct {
	Raw float64 `json:"raw"`
}

// Client talks to Yahoo. Some routes need a session cookie and a matching "crumb"; both are
// fetched on first use and again whenever Yahoo stops accepting them.
type Client struct {
	http  *http.Client
	mu    sync.Mutex
	crumb string
	cache cache
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil) // never fails with nil options
	return &Client{http: &http.Client{Timeout: FETCH_TIMEOUT, Jar: jar}, cache: cache{items: map[string]cached{}}}
}

func (c *Client) fetch(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", BROWSER_AGENT)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, MAX_BODY))
	return body, res.StatusCode, err
}

// session returns the crumb, opening a session first when there is none.
func (c *Client) session(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.crumb != "" {
		return c.crumb, nil
	}
	c.fetch(ctx, COOKIE_URL) // only the cookie matters; the status is always an error
	body, status, err := c.fetch(ctx, YAHOO_API+CRUMB_PATH)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK || len(body) == 0 {
		return "", fmt.Errorf("yahoo crumb: HTTP %d", status)
	}
	c.crumb = string(body)
	return c.crumb, nil
}

// get decodes one API route into out. Routes that need the crumb are retried once with a new
// session when Yahoo answers 401.
func (c *Client) get(ctx context.Context, path string, crumbed bool, out any) error {
	for attempt := 0; ; attempt++ {
		target := YAHOO_API + path
		if crumbed {
			crumb, err := c.session(ctx)
			if err != nil {
				return err
			}
			target += CRUMB_PARAM + url.QueryEscape(crumb)
		}
		body, status, err := c.fetch(ctx, target)
		if err != nil {
			return err
		}
		if status == http.StatusUnauthorized && crumbed && attempt == 0 {
			c.mu.Lock()
			c.crumb = ""
			c.mu.Unlock()
			continue
		}
		if status == http.StatusNotFound {
			return ErrNotFound
		}
		if status != http.StatusOK {
			return fmt.Errorf("yahoo: HTTP %d", status)
		}
		return json.Unmarshal(body, out)
	}
}

// Chart returns the candles for one range. An empty interval picks the range's default.
func (c *Client) Chart(ctx context.Context, symbol, rng, interval string) (Chart, error) {
	allowed := INTERVALS[rng]
	if interval == "" && len(allowed) > 0 {
		interval = allowed[0]
	}
	valid := false
	for _, a := range allowed {
		valid = valid || a == interval
	}
	if !valid || !SYMBOL.MatchString(symbol) {
		return Chart{}, ErrBadInput
	}
	out, err := c.candles(ctx, fmt.Sprintf(CHART_PATH, url.PathEscape(symbol), rng, interval, EXTENDED_RANGES[rng]))
	out.Range, out.Interval = rng, interval
	return out, err
}

// History returns the candles between two moments, for scoring a call made in the past. Yahoo
// keeps small candles only for a while, so the size steps up with the age of the start: five
// minutes inside HISTORY_5M, an hour inside HISTORY_1H, a day beyond that.
func (c *Client) History(ctx context.Context, symbol string, from, to time.Time) (Chart, error) {
	if !SYMBOL.MatchString(symbol) || !to.After(from) {
		return Chart{}, ErrBadInput
	}
	interval := "1d"
	switch age := time.Since(from); {
	case age < HISTORY_5M:
		interval = "5m"
	case age < HISTORY_1H:
		interval = "1h"
	}
	out, err := c.candles(ctx, fmt.Sprintf(HISTORY_PATH, url.PathEscape(symbol), from.Unix(), to.Unix(), interval))
	out.Interval = interval
	return out, err
}

// candles fetches and decodes one chart route.
func (c *Client) candles(ctx context.Context, path string) (Chart, error) {
	var res struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol             string  `json:"symbol"`
					Timezone           string  `json:"exchangeTimezoneName"`
					Price              float64 `json:"regularMarketPrice"`
					ChartPreviousClose float64 `json:"chartPreviousClose"`
					Periods            struct {
						Regular Session `json:"regular"`
					} `json:"currentTradingPeriod"`
				} `json:"meta"`
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open   []*float64 `json:"open"`
						High   []*float64 `json:"high"`
						Low    []*float64 `json:"low"`
						Close  []*float64 `json:"close"`
						Volume []*float64 `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := c.get(ctx, path, false, &res); err != nil {
		return Chart{}, err
	}
	if len(res.Chart.Result) == 0 {
		return Chart{}, ErrNotFound
	}
	r := res.Chart.Result[0]
	out := Chart{Symbol: r.Meta.Symbol, Timezone: r.Meta.Timezone, Price: r.Meta.Price,
		PreviousClose: r.Meta.ChartPreviousClose, Regular: r.Meta.Periods.Regular, Candles: []Candle{}}
	if len(r.Indicators.Quote) == 0 {
		return out, nil
	}
	q := r.Indicators.Quote[0]
	for i, t := range r.Timestamp {
		// A bar with no trades arrives as nulls. It is left out rather than drawn as a gap to zero.
		if i >= len(q.Close) || i >= len(q.Open) || i >= len(q.High) || i >= len(q.Low) || q.Close[i] == nil || q.Open[i] == nil || q.High[i] == nil || q.Low[i] == nil {
			continue
		}
		bar := Candle{T: t, O: *q.Open[i], H: *q.High[i], L: *q.Low[i], C: *q.Close[i]}
		if i < len(q.Volume) && q.Volume[i] != nil {
			bar.V = *q.Volume[i]
		}
		out.Candles = append(out.Candles, bar)
	}
	return out, nil
}

// Summary returns the quote, the key statistics, the company profile and the last four quarters.
func (c *Client) Summary(ctx context.Context, symbol string) (Summary, error) {
	if !SYMBOL.MatchString(symbol) {
		return Summary{}, ErrBadInput
	}
	var res struct {
		QuoteSummary struct {
			Result []struct {
				Price struct {
					Symbol         string `json:"symbol"`
					ShortName      string `json:"shortName"`
					LongName       string `json:"longName"`
					Exchange       string `json:"exchangeName"`
					Currency       string `json:"currency"`
					CurrencySymbol string `json:"currencySymbol"`
					Type           string `json:"quoteType"`
					MarketState    string `json:"marketState"`
					Delay          int    `json:"exchangeDataDelayedBy"`
					Price          raw    `json:"regularMarketPrice"`
					Change         raw    `json:"regularMarketChange"`
					ChangePct      raw    `json:"regularMarketChangePercent"`
					MarketTime     int64  `json:"regularMarketTime"`
					PreviousClose  raw    `json:"regularMarketPreviousClose"`
					Open           raw    `json:"regularMarketOpen"`
					DayHigh        raw    `json:"regularMarketDayHigh"`
					DayLow         raw    `json:"regularMarketDayLow"`
					Volume         raw    `json:"regularMarketVolume"`
					AvgVolume      raw    `json:"averageDailyVolume3Month"`
					MarketCap      raw    `json:"marketCap"`
					PrePrice       raw    `json:"preMarketPrice"`
					PreChange      raw    `json:"preMarketChange"`
					PreChangePct   raw    `json:"preMarketChangePercent"`
					PreTime        int64  `json:"preMarketTime"`
					PostPrice      raw    `json:"postMarketPrice"`
					PostChange     raw    `json:"postMarketChange"`
					PostChangePct  raw    `json:"postMarketChangePercent"`
					PostTime       int64  `json:"postMarketTime"`
				} `json:"price"`
				Detail struct {
					PE            raw `json:"trailingPE"`
					DividendYield raw `json:"dividendYield"`
					DividendRate  raw `json:"dividendRate"`
					High52        raw `json:"fiftyTwoWeekHigh"`
					Low52         raw `json:"fiftyTwoWeekLow"`
				} `json:"summaryDetail"`
				Profile struct {
					Summary     string `json:"longBusinessSummary"`
					Description string `json:"description"` // coins
					Website     string `json:"website"`
					Sector      string `json:"sector"`
					Industry    string `json:"industry"`
				} `json:"assetProfile"`
				Earnings struct {
					Chart struct {
						Quarterly []struct {
							Date     string `json:"date"`
							Actual   raw    `json:"actual"`
							Estimate raw    `json:"estimate"`
						} `json:"quarterly"`
						EarningsDate []raw `json:"earningsDate"`
					} `json:"earningsChart"`
					Financials struct {
						Quarterly []struct {
							Date     string `json:"date"`
							Revenue  raw    `json:"revenue"`
							Earnings raw    `json:"earnings"`
						} `json:"quarterly"`
					} `json:"financialsChart"`
				} `json:"earnings"`
			} `json:"result"`
		} `json:"quoteSummary"`
	}
	if err := c.get(ctx, fmt.Sprintf(SUMMARY_PATH, url.PathEscape(symbol)), true, &res); err != nil {
		return Summary{}, err
	}
	if len(res.QuoteSummary.Result) == 0 {
		return Summary{}, ErrNotFound
	}
	r := res.QuoteSummary.Result[0]
	p, d := r.Price, r.Detail
	out := Summary{
		Symbol: p.Symbol, Name: p.LongName, Exchange: p.Exchange, Currency: p.Currency, CurrencySymbol: p.CurrencySymbol,
		Type: p.Type, MarketState: p.MarketState, DelayMinutes: p.Delay,
		Price: p.Price.Raw, Change: p.Change.Raw, ChangePct: p.ChangePct.Raw * PERCENT, MarketTime: p.MarketTime,
		PreviousClose: p.PreviousClose.Raw, Open: p.Open.Raw, DayHigh: p.DayHigh.Raw, DayLow: p.DayLow.Raw,
		Volume: p.Volume.Raw, AvgVolume: p.AvgVolume.Raw, MarketCap: p.MarketCap.Raw,
		PE: d.PE.Raw, DividendPct: d.DividendYield.Raw * PERCENT, DividendRate: d.DividendRate.Raw, High52: d.High52.Raw, Low52: d.Low52.Raw,
		About: r.Profile.Summary, Sector: r.Profile.Sector, Industry: r.Profile.Industry, Quarters: []Quarter{},
	}
	if out.Name == "" {
		out.Name = p.ShortName
	}
	if out.About == "" {
		out.About = r.Profile.Description
	}
	if strings.HasPrefix(r.Profile.Website, HTTPS_PREFIX) {
		out.Website = r.Profile.Website
	}
	switch {
	case p.MarketState == STATE_PRE && p.PrePrice.Raw > 0:
		out.ExtSession, out.ExtPrice, out.ExtChange, out.ExtChangePct, out.ExtTime = SESSION_PRE, p.PrePrice.Raw, p.PreChange.Raw, p.PreChangePct.Raw*PERCENT, p.PreTime
	case p.MarketState != STATE_REGULAR && p.PostPrice.Raw > 0:
		out.ExtSession, out.ExtPrice, out.ExtChange, out.ExtChangePct, out.ExtTime = SESSION_POST, p.PostPrice.Raw, p.PostChange.Raw, p.PostChangePct.Raw*PERCENT, p.PostTime
	}
	eps := map[string][2]float64{}
	for _, q := range r.Earnings.Chart.Quarterly {
		eps[q.Date] = [2]float64{q.Actual.Raw, q.Estimate.Raw}
	}
	for _, q := range r.Earnings.Financials.Quarterly {
		out.Quarters = append(out.Quarters, Quarter{Label: q.Date, Revenue: q.Revenue.Raw, Earnings: q.Earnings.Raw, EPS: eps[q.Date][0], EPSEstimate: eps[q.Date][1]})
	}
	if dates := r.Earnings.Chart.EarningsDate; len(dates) > 0 {
		out.NextEarnings = int64(dates[0].Raw)
	}
	return out, nil
}

// Search finds tickers by name or symbol and the newest stories about them.
func (c *Client) Search(ctx context.Context, query string) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > MAX_QUERY_CHARS {
		return SearchResult{}, ErrBadInput
	}
	var res struct {
		Quotes []struct {
			Symbol    string `json:"symbol"`
			ShortName string `json:"shortname"`
			LongName  string `json:"longname"`
			Exchange  string `json:"exchDisp"`
			Type      string `json:"typeDisp"`
		} `json:"quotes"`
		News []struct {
			Title     string `json:"title"`
			Publisher string `json:"publisher"`
			Link      string `json:"link"`
			At        int64  `json:"providerPublishTime"`
			Thumbnail struct {
				Resolutions []struct {
					URL   string `json:"url"`
					Width int    `json:"width"`
				} `json:"resolutions"`
			} `json:"thumbnail"`
		} `json:"news"`
	}
	if err := c.get(ctx, fmt.Sprintf(SEARCH_PATH, url.QueryEscape(query), SEARCH_QUOTES, SEARCH_NEWS), false, &res); err != nil {
		return SearchResult{}, err
	}
	out := SearchResult{Matches: []Match{}, News: []Story{}}
	for _, q := range res.Quotes {
		if !SYMBOL.MatchString(q.Symbol) {
			continue
		}
		name := q.LongName
		if name == "" {
			name = q.ShortName
		}
		out.Matches = append(out.Matches, Match{Symbol: q.Symbol, Name: name, Exchange: q.Exchange, Type: q.Type})
	}
	for _, n := range res.News {
		if !strings.HasPrefix(n.Link, HTTPS_PREFIX) {
			continue // the page links to these, so nothing but https gets through
		}
		story := Story{Title: n.Title, Publisher: n.Publisher, URL: n.Link, At: n.At}
		best := 0
		for _, t := range n.Thumbnail.Resolutions {
			// The smallest picture that is still wide enough; failing that, the widest there is.
			wide, bestWide := t.Width >= THUMB_MIN_WIDTH, best >= THUMB_MIN_WIDTH
			better := best == 0 || wide && (!bestWide || t.Width < best) || !wide && !bestWide && t.Width > best
			if better && strings.HasPrefix(t.URL, HTTPS_PREFIX) {
				story.Thumb, best = t.URL, t.Width
			}
		}
		out.News = append(out.News, story)
	}
	return out, nil
}

// Ticks returns short quotes for a list of symbols, in Yahoo's order.
func (c *Client) Ticks(ctx context.Context, symbols []string) ([]Tick, error) {
	if len(symbols) == 0 || len(symbols) > MAX_QUOTES {
		return nil, ErrBadInput
	}
	for _, s := range symbols {
		if !SYMBOL.MatchString(s) {
			return nil, ErrBadInput
		}
	}
	var res struct {
		QuoteResponse struct {
			Result []struct {
				Symbol    string  `json:"symbol"`
				ShortName string  `json:"shortName"`
				LongName  string  `json:"longName"`
				Price     float64 `json:"regularMarketPrice"`
				ChangePct float64 `json:"regularMarketChangePercent"`
				Currency  string  `json:"currency"`
			} `json:"result"`
		} `json:"quoteResponse"`
	}
	if err := c.get(ctx, fmt.Sprintf(QUOTES_PATH, url.QueryEscape(strings.Join(symbols, ","))), true, &res); err != nil {
		return nil, err
	}
	out := []Tick{}
	for _, q := range res.QuoteResponse.Result {
		name := q.LongName
		if name == "" {
			name = q.ShortName
		}
		out = append(out, Tick{Symbol: q.Symbol, Name: name, Price: q.Price, ChangePct: q.ChangePct, Currency: q.Currency})
	}
	return out, nil
}

// Related returns short quotes for the tickers people most often look at beside this one.
func (c *Client) Related(ctx context.Context, symbol string) ([]Tick, error) {
	if !SYMBOL.MatchString(symbol) {
		return nil, ErrBadInput
	}
	var res struct {
		Finance struct {
			Result []struct {
				Recommended []struct {
					Symbol string `json:"symbol"`
				} `json:"recommendedSymbols"`
			} `json:"result"`
		} `json:"finance"`
	}
	if err := c.get(ctx, fmt.Sprintf(RELATED_PATH, url.PathEscape(symbol)), false, &res); err != nil {
		return nil, err
	}
	symbols := []string{}
	for _, r := range res.Finance.Result {
		for _, s := range r.Recommended {
			if SYMBOL.MatchString(s.Symbol) && len(symbols) < MAX_QUOTES {
				symbols = append(symbols, s.Symbol)
			}
		}
	}
	if len(symbols) == 0 {
		return []Tick{}, nil
	}
	return c.Ticks(ctx, symbols)
}
