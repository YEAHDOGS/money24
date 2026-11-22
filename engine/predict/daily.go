package predict

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	// Daily counts: Kalshi markets that settle on a number an agency publishes once a day,
	// where much of the answer is already public. AAA publishes the US average price of
	// regular gas and of diesel every morning (the settlement source for KXAAAGASD,
	// KXAAAGASW, KXDIESELD and KXDIESELW: the weekly-labelled ones settle on one named
	// day's AAA value too). The TSA publishes yesterday's checkpoint count every day
	// (the settlement source for KXTSAD). The model is deliberately plain: the settling
	// value is centred on the latest published value, with a normal spread built from
	// the recent day-to-day moves, widened by the square root of the days still to run.
	// When the settling date is already published, the answer is known, not forecast.
	// Series dropped after checking Kalshi's live series list and rules: KXTSA (the
	// older duplicate of KXTSAD, with no open markets), KXTSAW (settles on a weekly
	// average whose averaging window the rules never define, and no volume), the fifty
	// state gas series KXAAAGASCD* (same model state by state; the desk starts with
	// the national numbers the task named), and the EIA weekly series (a different,
	// weekly number from the AAA daily value Kalshi settles on).
	KIND_DAILY          = "dailycounts"
	DAILY_NAME          = "Daily published counts"
	DAILY_ABOUT         = "AAA daily gas and diesel averages and TSA daily checkpoint counts: the latest published value plus the spread of recent daily moves, or the published answer itself when the settling date is already out."
	DAILY_KNOWN_ID      = "daily-known"
	DAILY_KNOWN_NAME    = "Daily counts: already published"
	DAILY_FORECAST_ID   = "daily-forecast"
	DAILY_FORECAST_NAME = "Daily counts: next publication"

	SERIES_AAA_GAS_DAILY     = "KXAAAGASD"
	SERIES_AAA_GAS_WEEKLY    = "KXAAAGASW"
	SERIES_AAA_DIESEL_DAILY  = "KXDIESELD"
	SERIES_AAA_DIESEL_WEEKLY = "KXDIESELW"
	SERIES_TSA_DAILY         = "KXTSAD"

	STRIKE_GREATER_OR_EQUAL = "greater_or_equal"

	LABEL_AAA_REGULAR = "AAA regular"
	LABEL_AAA_DIESEL  = "AAA diesel"
	LABEL_TSA         = "TSA screened"

	DEFAULT_DAILY_SIGMA_FLOOR_FRAC = 0.004 // the least daily spread, as a share of the latest value
	DEFAULT_DAILY_LOOKBACK         = 28    // day-to-day moves the spread is built from
	DEFAULT_DAILY_MAX_GAP_DAYS     = 4     // the most days past the latest publication a forecast may reach

	DAILY_REASON_P      = "model %.0f%% YES"
	DAILY_REASON_KNOWN  = "already published: %s on %s"
	DAILY_REASON_LATEST = "latest %s on %s"
	DAILY_REASON_SIGMA  = "daily moves ±%s, %d day(s) to the settling date"
	DAILY_PRICE_FORMAT  = "$%.4f"
	DAILY_COUNT_FORMAT  = "%.0f"
	DAILY_DATE_FORMAT   = "Jan 2"
)

// DAILY_FORECAST_SERIES are the series the forecast strategy trades: prices that drift a little a day.
// TSA counts swing by weekday and holiday far more than a day-to-day spread captures, so until a
// weekday-aware model exists they trade only once published.
var DAILY_FORECAST_SERIES = []string{SERIES_AAA_GAS_DAILY, SERIES_AAA_GAS_WEEKLY, SERIES_AAA_DIESEL_DAILY, SERIES_AAA_DIESEL_WEEKLY}

// DailyParams are the daily-counts kind's own settings.
type DailyParams struct {
	Only           []string `json:"only,omitempty"` // series tickers to trade; empty: all five
	KnownOnly      bool     `json:"known_only"`     // take only markets whose settling date is already published
	SigmaFloorFrac float64  `json:"sigma_floor_frac"`
	Lookback       int      `json:"lookback"`
	MaxGapDays     int      `json:"max_gap_days"`
}

type dailySeriesInfo struct {
	source string
	label  string
	money  bool // prices print as $x.xxxx; counts as whole numbers
}

var dailySeries = map[string]dailySeriesInfo{
	SERIES_AAA_GAS_DAILY:     {source: SOURCE_AAA_REGULAR, label: LABEL_AAA_REGULAR, money: true},
	SERIES_AAA_GAS_WEEKLY:    {source: SOURCE_AAA_REGULAR, label: LABEL_AAA_REGULAR, money: true},
	SERIES_AAA_DIESEL_DAILY:  {source: SOURCE_AAA_DIESEL, label: LABEL_AAA_DIESEL, money: true},
	SERIES_AAA_DIESEL_WEEKLY: {source: SOURCE_AAA_DIESEL, label: LABEL_AAA_DIESEL, money: true},
	SERIES_TSA_DAILY:         {source: SOURCE_TSA, label: LABEL_TSA, money: false},
}

type dailyCounts struct {
	p      DailyParams
	series map[string]dailySeriesInfo
	data   *dailyData
}

func init() {
	Register(Kind{ID: KIND_DAILY, Name: DAILY_NAME, About: DAILY_ABOUT, New: newDailyCounts})
}

func newDailyCounts(raw json.RawMessage) (Strategy, error) {
	p := DailyParams{SigmaFloorFrac: DEFAULT_DAILY_SIGMA_FLOOR_FRAC, Lookback: DEFAULT_DAILY_LOOKBACK, MaxGapDays: DEFAULT_DAILY_MAX_GAP_DAYS}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	if p.SigmaFloorFrac < 0 || p.Lookback < 1 || p.MaxGapDays < 0 {
		return nil, errors.New("sigma_floor_frac must not be negative, lookback at least 1, max_gap_days not negative")
	}
	only := map[string]bool{}
	for _, ticker := range p.Only {
		only[strings.ToUpper(ticker)] = true
	}
	d := &dailyCounts{p: p, series: map[string]dailySeriesInfo{}, data: sharedDaily}
	for ticker, info := range dailySeries {
		if len(only) > 0 && !only[ticker] {
			continue
		}
		d.series[ticker] = info
	}
	if len(d.series) == 0 {
		return nil, errors.New("no known daily series selected")
	}
	return d, nil
}

func (d *dailyCounts) Scope() Scope {
	out := make([]string, 0, len(d.series))
	for ticker := range d.series {
		out = append(out, ticker)
	}
	sort.Strings(out)
	return Scope{Series: out}
}

func (d *dailyCounts) Prepare(ctx context.Context, env *Env, markets []Market, now time.Time) error {
	needed := map[string]bool{}
	for _, m := range markets {
		if info, ok := d.series[m.Series()]; ok {
			needed[info.source] = true
		}
	}
	var errs []error
	for source := range needed {
		if _, err := d.data.load(ctx, env, source, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// dailyEstimate is the model for one settling date: either the published value itself,
// or the latest published value with a normal spread for the days still to run.
type dailyEstimate struct {
	known      bool
	knownValue float64
	mean       float64 // the latest published value
	sigma      float64 // the spread over the whole gap, not per day
	latest     DailyPoint
	gapDays    int
}

// estimate builds the model for target from one source's published history.
func estimate(points []DailyPoint, target time.Time, lookback int, floorFrac float64) (dailyEstimate, bool) {
	if len(points) == 0 {
		return dailyEstimate{}, false
	}
	sorted := append([]DailyPoint{}, points...)
	sortPoints(sorted)
	snap := &DailySnapshot{Points: sorted}
	if value, ok := snap.On(target); ok {
		return dailyEstimate{known: true, knownValue: value}, true
	}
	latest, ok := snap.Latest(target)
	if !ok {
		return dailyEstimate{}, false
	}
	gapDays := int(math.Round(target.Sub(latest.Date).Hours() / DATE_ONLY_HOURS))
	if gapDays < 1 {
		return dailyEstimate{}, false
	}
	// The spread: the typical size of a day-to-day move over the recent points, each
	// move between points g days apart scaled down by sqrt(g) to a one-day move.
	from := 0
	if len(sorted) > lookback+1 {
		from = len(sorted) - lookback - 1
	}
	recent := sorted[from:]
	sumSquares := 0.0
	moves := 0
	for i := 1; i < len(recent); i++ {
		gap := recent[i].Date.Sub(recent[i-1].Date).Hours() / DATE_ONLY_HOURS
		if gap < 1 {
			continue
		}
		move := (recent[i].Value - recent[i-1].Value) / math.Sqrt(gap)
		sumSquares += move * move
		moves++
	}
	sigmaDaily := 0.0
	if moves > 0 {
		sigmaDaily = math.Sqrt(sumSquares / float64(moves))
	}
	if floor := floorFrac * math.Abs(latest.Value); sigmaDaily < floor {
		sigmaDaily = floor
	}
	if sigmaDaily <= 0 {
		return dailyEstimate{}, false
	}
	est := dailyEstimate{mean: latest.Value, latest: latest, gapDays: gapDays}
	est.sigma = sigmaDaily * math.Sqrt(float64(gapDays))
	return est, true
}

// yesHolds says whether a settled value makes YES win. Thresholds are exact published
// values here, never whole-degree ranges, so no half-unit offset is added or removed:
// a "greater" market on $3.7000 is won by $3.7001, not by $4.2000.
func yesHolds(value float64, m Market) (bool, bool) {
	switch m.Strike {
	case STRIKE_GREATER:
		return value > m.Floor, true
	case STRIKE_GREATER_OR_EQUAL:
		return value >= m.Floor, true
	case STRIKE_LESS:
		return value < m.Cap, true
	case STRIKE_BETWEEN:
		return value >= m.Floor && value <= m.Cap, m.Cap >= m.Floor
	}
	return false, false
}

// yesProb is the chance YES wins for an estimate: certain when the date is published,
// otherwise the estimate's normal spread read against the market's threshold. It is
// the range math of Market.Range translated to exact thresholds, in probability form:
// P(above floor), P(below cap), or P(between floor and cap).
func yesProb(est dailyEstimate, m Market) (float64, bool) {
	if est.known {
		holds, ok := yesHolds(est.knownValue, m)
		if !ok {
			return 0, false
		}
		if holds {
			return DOLLAR, true
		}
		return 0, true
	}
	if est.sigma <= 0 {
		return 0, false
	}
	switch m.Strike {
	case STRIKE_GREATER, STRIKE_GREATER_OR_EQUAL:
		return DOLLAR - normalCDF((m.Floor-est.mean)/est.sigma), true
	case STRIKE_LESS:
		return normalCDF((m.Cap - est.mean) / est.sigma), true
	case STRIKE_BETWEEN:
		if m.Cap < m.Floor {
			return 0, false
		}
		prob := normalCDF((m.Cap-est.mean)/est.sigma) - normalCDF((m.Floor-est.mean)/est.sigma)
		return math.Max(0, prob), true
	}
	return 0, false
}

// formatValue prints one published value the way its source prints it.
func formatValue(info dailySeriesInfo, value float64) string {
	if info.money {
		return fmt.Sprintf(DAILY_PRICE_FORMAT, value)
	}
	text := fmt.Sprintf(DAILY_COUNT_FORMAT, value)
	whole := text
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign, whole = "-", strings.TrimPrefix(text, "-")
	}
	parts := []string{}
	for len(whole) > 3 {
		parts = append([]string{whole[len(whole)-3:]}, parts...)
		whole = whole[:len(whole)-3]
	}
	parts = append([]string{whole}, parts...)
	return sign + strings.Join(parts, ",")
}

func (d *dailyCounts) Evaluate(m Market, now time.Time) (Opinion, bool) {
	info, ok := d.series[m.Series()]
	if !ok {
		return Opinion{}, false
	}
	target, err := m.EventDate()
	if err != nil {
		return Opinion{}, false
	}
	target = dayUTC(target)
	snap := d.data.get(info.source)
	if snap == nil {
		return Opinion{}, false
	}
	est, ok := estimate(snap.Points, target, d.p.Lookback, d.p.SigmaFloorFrac)
	if !ok {
		return Opinion{}, false
	}
	if d.p.KnownOnly && !est.known {
		return Opinion{}, false
	}
	if !est.known && est.gapDays > d.p.MaxGapDays {
		return Opinion{}, false
	}
	prob, ok := yesProb(est, m)
	if !ok {
		return Opinion{}, false
	}
	side := SIDE_YES
	if (DOLLAR-prob)-m.NoAsk > prob-m.YesAsk {
		side = SIDE_NO
	}
	reasons := []string{fmt.Sprintf(DAILY_REASON_P, prob*PERCENT)}
	if est.known {
		reasons = append(reasons, fmt.Sprintf(DAILY_REASON_KNOWN, formatValue(info, est.knownValue), target.Format(DAILY_DATE_FORMAT)))
		return Opinion{Side: side, Prob: prob, Reasons: reasons}, true
	}
	reasons = append(reasons, fmt.Sprintf(DAILY_REASON_LATEST, formatValue(info, est.latest.Value), est.latest.Date.Format(DAILY_DATE_FORMAT)))
	perDay := est.sigma / math.Sqrt(float64(est.gapDays))
	reasons = append(reasons, fmt.Sprintf(DAILY_REASON_SIGMA, formatValue(info, perDay), est.gapDays))
	return Opinion{Side: side, Prob: prob, Reasons: reasons}, true
}

// dailyParams builds the settings; only lists the series traded, empty for all.
func dailyParams(knownOnly bool, only []string) json.RawMessage {
	raw, _ := json.Marshal(DailyParams{Only: only, KnownOnly: knownOnly, SigmaFloorFrac: DEFAULT_DAILY_SIGMA_FLOOR_FRAC,
		Lookback: DEFAULT_DAILY_LOOKBACK, MaxGapDays: DEFAULT_DAILY_MAX_GAP_DAYS})
	return raw
}

func defaultDailyKnown() Config {
	return Config{ID: DAILY_KNOWN_ID, Kind: KIND_DAILY, Name: DAILY_KNOWN_NAME, On: true, Risk: RISK_LOW, EveryMinutes: DEFAULT_EVERY_MINUTES,
		Entry:  Entry{MinPrice: 0.50, MaxPrice: 0.97, MaxSpread: 0.05, LeftMinutes: DEFAULT_LEFT_MINUTES, MinEdge: 0.03, MinProb: 0.95},
		Limits: Limits{StakeUSD: 15, MaxOpen: 15, NewPerPass: 5, DailyStakeUSD: 150}, Params: dailyParams(true, nil)}
}

func defaultDailyForecast() Config {
	return Config{ID: DAILY_FORECAST_ID, Kind: KIND_DAILY, Name: DAILY_FORECAST_NAME, On: true, Risk: RISK_MEDIUM, EveryMinutes: DEFAULT_EVERY_MINUTES * 2,
		Entry:  Entry{MinPrice: 0.08, MaxPrice: 0.92, MaxSpread: 0.06, LeftMinutes: 30, MinEdge: 0.08, MaxEdge: 0.25},
		Limits: Limits{StakeUSD: 10, MaxOpen: 20, NewPerPass: 4, DailyStakeUSD: 100}, Params: dailyParams(false, DAILY_FORECAST_SERIES)}
}
