package plays

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/YEAHDOGS/money24/engine/market"
	"github.com/YEAHDOGS/money24/engine/signals"
)

const (
	PLATFORM_ROBINHOOD = "robinhood"
	PLATFORM_WEBULL    = "webull"
	PLATFORM_TEXT      = "text"
	ROBINHOOD_API      = "https://api.robinhood.com/options/instruments/%s/"
	LINK_TIMEOUT       = 10 * time.Second
	MAX_LINK_BODY      = 1 << 20
	OCC_STRIKE_SCALE   = 1000
	CHART_RANGE        = "6mo"
	CHART_INTERVAL     = "1d"
	SOURCE_YAHOO       = "Yahoo Finance (delayed)"
	TICKER_MAX         = 6
	CENTURY            = 100
	CENTURY_START      = 2000
)

var (
	ErrUnknownLink = errors.New("not a Robinhood or Webull contract or stock link, an option symbol, or a readable callout")
	// robinhood.com/options/<instrument id>_L1 (a long strategy code) or /options/<instrument id>
	ROBINHOOD_OPTION = regexp.MustCompile(`(?i)robinhood\.com/options/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)
	ROBINHOOD_STOCK  = regexp.MustCompile(`(?i)robinhood\.com/(?:us/en/)?stocks/([A-Z.]{1,6})\b`)
	// webull.com/quote/nasdaq-aal, webull.com/quote/nyse-dal/options
	WEBULL_QUOTE = regexp.MustCompile(`(?i)webull\.com/quote/[a-z]+-([a-z.]{1,6})\b`)
	// An OCC option symbol anywhere: AAL261120P00012500
	OCC_SYMBOL = regexp.MustCompile(`\b([A-Z]{1,6})(\d{2})(\d{2})(\d{2})([CP])(\d{8})\b`)
	// Plain contract text: "AAL 12.5P 11/20", "$DAL 80 put 11/27/26", "SPY 750 calls 12/18"
	PLAIN_CONTRACT = regexp.MustCompile(`(?i)^\$?([A-Z]{1,6})\s+\$?(\d+(?:\.\d+)?)\s*(c|p|calls?|puts?)\s+(\d{1,2})/(\d{1,2})(?:/(\d{2,4}))?\b`)
)

// Link is one pasted URL or callout and what came of it. The history keeps every one.
type Link struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	URL      string    `json:"url"`
	Platform string    `json:"platform"`
	Contract Contract  `json:"contract"`
	PlayID   string    `json:"play_id,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Resolver turns links into contracts and fills in the market behind a play.
type Resolver struct {
	HTTP   *http.Client
	Market *market.Client
}

// Resolve reads a pasted link or text into a contract and names the platform it came from.
func (r Resolver) Resolve(ctx context.Context, input string) (Contract, string, error) {
	input = strings.TrimSpace(input)
	if m := ROBINHOOD_OPTION.FindStringSubmatch(input); m != nil {
		c, err := r.robinhoodOption(ctx, m[1])
		return c, PLATFORM_ROBINHOOD, err
	}
	platform := PLATFORM_TEXT
	switch {
	case strings.Contains(strings.ToLower(input), "robinhood.com"):
		platform = PLATFORM_ROBINHOOD
	case strings.Contains(strings.ToLower(input), "webull.com"):
		platform = PLATFORM_WEBULL
	}
	if m := OCC_SYMBOL.FindStringSubmatch(strings.ToUpper(input)); m != nil {
		strike, _ := strconv.ParseFloat(m[6], 64)
		return Contract{Symbol: m[1], Market: MARKET_OPTION, Right: m[5], Strike: strike / OCC_STRIKE_SCALE,
			Expiry: fmt.Sprintf("20%s-%s-%s", m[2], m[3], m[4])}, platform, nil
	}
	if m := ROBINHOOD_STOCK.FindStringSubmatch(input); m != nil {
		return Contract{Symbol: strings.ToUpper(m[1]), Market: MARKET_STOCK}, platform, nil
	}
	if m := WEBULL_QUOTE.FindStringSubmatch(input); m != nil {
		return Contract{Symbol: strings.ToUpper(m[1]), Market: MARKET_STOCK}, platform, nil
	}
	if m := PLAIN_CONTRACT.FindStringSubmatch(input); m != nil {
		if c, ok := plainContract(m, time.Now()); ok {
			return c, platform, nil
		}
	}
	// Anything else goes to the callout parser: "BTO $SPY 750P 11/20 @ 1.20".
	sig := signals.Parse(input, time.Now())
	if sig.Symbol == "" || sig.Address != "" {
		return Contract{}, platform, ErrUnknownLink
	}
	if sig.Right == "" {
		return Contract{Symbol: sig.Symbol, Market: MARKET_STOCK}, platform, nil
	}
	return Contract{Symbol: sig.Symbol, Market: MARKET_OPTION, Right: sig.Right, Strike: sig.Strike, Expiry: sig.Expiry}, platform, nil
}

// robinhoodOption reads Robinhood's public instrument record: no login, contract terms only.
func (r Resolver) robinhoodOption(ctx context.Context, id string) (Contract, error) {
	ctx, cancel := context.WithTimeout(ctx, LINK_TIMEOUT)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(ROBINHOOD_API, url.PathEscape(id)), nil)
	if err != nil {
		return Contract{}, err
	}
	res, err := r.HTTP.Do(req)
	if err != nil {
		return Contract{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Contract{}, fmt.Errorf("robinhood instrument %s: HTTP %d", id, res.StatusCode)
	}
	var in struct {
		Symbol string `json:"chain_symbol"`
		Expiry string `json:"expiration_date"`
		Strike string `json:"strike_price"`
		Type   string `json:"type"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, MAX_LINK_BODY)).Decode(&in); err != nil {
		return Contract{}, err
	}
	strike, err := strconv.ParseFloat(in.Strike, 64)
	if err != nil || in.Symbol == "" {
		return Contract{}, fmt.Errorf("robinhood instrument %s: unreadable", id)
	}
	right := RIGHT_CALL
	if strings.EqualFold(in.Type, "put") {
		right = RIGHT_PUT
	}
	return Contract{Symbol: strings.ToUpper(in.Symbol), Market: MARKET_OPTION, Right: right, Strike: strike, Expiry: in.Expiry}, nil
}

// Snapshot loads the market behind a contract: the quote, open interest and volume, implied
// volatility, the next earnings date, and the 20/50-day averages and RSI from daily candles.
// What fails is noted on the snapshot instead of failing the play.
func (r Resolver) Snapshot(ctx context.Context, c Contract, now time.Time) Market {
	m := Market{Source: SOURCE_YAHOO, At: now.UTC()}
	notes := []string{}
	if c.Market == MARKET_OPTION {
		q, err := r.Market.Option(ctx, c.Symbol, c.Expiry, c.Right, c.Strike)
		if err != nil {
			notes = append(notes, "option quote: "+err.Error())
		} else {
			m.Bid, m.Ask, m.Last, m.IV = q.Bid, q.Ask, q.Last, q.IV
			m.OpenInterest, m.Volume, m.Contract = q.OpenInterest, q.Volume, q.Contract
			m.Spot, m.PreviousClose, m.DayVolume, m.AvgVolume = q.Spot, q.PreviousClose, q.DayVolume, q.AvgVolume
			if q.NextEarnings > 0 {
				m.NextEarnings = time.Unix(q.NextEarnings, 0).UTC()
			}
		}
	}
	chart, err := r.Market.Chart(ctx, c.Symbol, CHART_RANGE, CHART_INTERVAL)
	if err != nil {
		notes = append(notes, "candles: "+err.Error())
	} else {
		closes := make([]float64, 0, len(chart.Candles))
		volume := 0.0
		for _, k := range chart.Candles {
			closes, volume = append(closes, k.C), volume+k.V
		}
		m.MA20, m.MA50, m.RSI14 = sma(closes, MA_SHORT), sma(closes, MA_LONG), rsi(closes, RSI_PERIOD)
		if m.Spot <= 0 {
			m.Spot, m.PreviousClose = chart.Price, chart.PreviousClose
		}
		if c.Market == MARKET_STOCK {
			m.IV = historicalVol(closes, HV_PERIOD)
			m.Last = chart.Price
			if n := len(chart.Candles); n > 0 {
				m.DayVolume = chart.Candles[n-1].V
				m.Volume = int(m.DayVolume)
				m.AvgVolume = volume / float64(n)
			}
		}
	}
	if c.Market == MARKET_STOCK && m.Spot > 0 {
		// Yahoo's chart has no book: a share play is planned at the last price, a cent either side.
		m.Bid, m.Ask = m.Spot-PRICE_TICK, m.Spot+PRICE_TICK
	}
	m.Note = strings.Join(notes, "; ")
	return m
}

const PRICE_TICK = 0.01

// autoEvidence turns the snapshot into evidence lines, so the play shows who else is in it.
func autoEvidence(m Market, bullish bool) []Evidence {
	out := []Evidence{}
	at := m.At
	if m.OpenInterest > 0 {
		out = append(out, Evidence{Kind: EV_INTEREST, Label: fmt.Sprintf("%d contracts open on this strike", m.OpenInterest),
			Value: float64(m.OpenInterest), Stance: STANCE_NEUTRAL, Source: m.Source, Verified: true, Auto: true, At: at})
	}
	if m.AvgVolume > 0 && m.DayVolume > 0 {
		rel := m.DayVolume / m.AvgVolume
		out = append(out, Evidence{Kind: EV_VOLUME, Label: fmt.Sprintf("Stock volume %.2fx its average (%s vs %s)", rel, compact(m.DayVolume), compact(m.AvgVolume)),
			Value: rel, Stance: STANCE_NEUTRAL, Source: m.Source, Verified: true, Auto: true, At: at})
	}
	if m.RSI14 > 0 {
		stance := STANCE_NEUTRAL
		switch {
		case m.RSI14 >= RSI_HIGH:
			stance = map[bool]string{true: STANCE_AGAINST, false: STANCE_FOR}[bullish]
		case m.RSI14 <= RSI_LOW:
			stance = map[bool]string{true: STANCE_FOR, false: STANCE_AGAINST}[bullish]
		}
		out = append(out, Evidence{Kind: EV_TECHNICAL, Label: fmt.Sprintf("RSI(14) %.0f", m.RSI14), Value: m.RSI14, Stance: stance,
			Source: m.Source, Verified: true, Auto: true, At: at})
	}
	if m.MA20 > 0 && m.MA50 > 0 && m.Spot > 0 {
		out = append(out, Evidence{Kind: EV_TECHNICAL, Label: fmt.Sprintf("Price %.2f vs 20-day %.2f and 50-day %.2f", m.Spot, m.MA20, m.MA50),
			Stance: STANCE_NEUTRAL, Source: m.Source, Verified: true, Auto: true, At: at})
	}
	return out
}

// compact writes 54305947 as 54.3M.
func compact(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.1fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.1fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1fK", v/1e3)
	}
	return fmt.Sprintf("%.0f", v)
}

// Draft builds a complete play from a contract, the market and the template: entry at the mid,
// the stop, the ladder, the runner and every failsafe, so it can paper trade the moment it exists.
func Draft(c Contract, m Market, t Template, now time.Time) Play {
	p := Play{CreatedAt: now.UTC(), UpdatedAt: now.UTC(), Status: STATUS_DRAFT, Contract: c, Market: &m}
	p.Title = c.Symbol
	if c.Market == MARKET_OPTION {
		p.Title = fmt.Sprintf("%s %g%s %s", c.Symbol, c.Strike, c.Right, c.Expiry)
	}
	entry := m.Last
	if m.Bid > 0 && m.Ask > 0 {
		entry = (m.Bid + m.Ask) / 2
	}
	entry = math.Round(entry*CENT) / CENT
	p.Entry = entry
	mult := p.Multiplier()
	if entry > 0 {
		p.Qty = math.Max(1, math.Floor(t.BudgetUSD/(entry*mult)))
	}
	p.Stop = math.Max(PRICE_TICK, math.Round(entry*(1-t.StopPct/PERCENT)*CENT)/CENT)
	left := p.Qty
	for _, r := range t.Ladder {
		n := math.Min(left, math.Round(p.Qty*r.PctOfPosition/PERCENT))
		if n <= 0 {
			continue
		}
		p.Ladder = append(p.Ladder, Rung{Qty: n, Price: math.Round(entry*(1+r.GainPct/PERCENT)*CENT) / CENT})
		left -= n
	}
	if len(p.Ladder) == 0 && len(t.Ladder) > 0 && p.Qty > 0 {
		// One unit: it all exits at the first rung, so the play still has a take-profit.
		p.Ladder = []Rung{{Qty: p.Qty, Price: math.Round(entry*(1+t.Ladder[0].GainPct/PERCENT)*CENT) / CENT}}
		left = 0
	}
	p.Runner = Runner{Qty: left}
	if left > 0 {
		p.Runner.Target = math.Round(entry*(1+t.RunnerGainPct/PERCENT)*CENT) / CENT
	}
	p.Failsafes = Failsafes{SidewaysMinutes: t.SidewaysMinutes, SidewaysMinGainPct: t.SidewaysMinGainPct,
		NoQuoteMinutes: t.NoQuoteMinutes, GapPct: t.GapPct, Calamity: t.Calamity}
	if expiry, ok := expiryClose(c.Expiry); ok {
		day := expiry.AddDate(0, 0, -t.ExitDaysBeforeExpiry)
		// Back off the weekend: an exit day on Saturday or Sunday moves to Friday.
		for day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			day = day.AddDate(0, 0, -1)
		}
		p.Failsafes.ExitBy = time.Date(day.Year(), day.Month(), day.Day(), t.ExitHour, 0, 0, 0, MARKET_TZ).UTC()
	} else if t.HoldDays > 0 {
		p.Failsafes.ExitBy = now.AddDate(0, 0, t.HoldDays).UTC()
	}
	p.Evidence = autoEvidence(m, c.Right != RIGHT_PUT)
	return p
}

// plainContract reads a PLAIN_CONTRACT match. With no year, the expiry is the next such date.
func plainContract(m []string, now time.Time) (Contract, bool) {
	strike, err := strconv.ParseFloat(m[2], 64)
	month, _ := strconv.Atoi(m[4])
	day, _ := strconv.Atoi(m[5])
	if err != nil || month < 1 || month > 12 || day < 1 || day > 31 {
		return Contract{}, false
	}
	today := now.In(MARKET_TZ)
	year := today.Year()
	if m[6] != "" {
		year, _ = strconv.Atoi(m[6])
		if year < CENTURY {
			year += CENTURY_START
		}
	}
	expiry := time.Date(year, time.Month(month), day, 0, 0, 0, 0, MARKET_TZ)
	if m[6] == "" && expiry.Before(time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, MARKET_TZ)) {
		expiry = expiry.AddDate(1, 0, 0)
	}
	right := RIGHT_CALL
	if strings.HasPrefix(strings.ToLower(m[3]), "p") {
		right = RIGHT_PUT
	}
	return Contract{Symbol: strings.ToUpper(m[1]), Market: MARKET_OPTION, Right: right, Strike: strike, Expiry: expiry.Format(EXPIRY_LAYOUT)}, true
}
