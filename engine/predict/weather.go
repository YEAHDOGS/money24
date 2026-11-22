package predict

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

const (
	// Weather: Kalshi's daily high and low temperature markets, one per city and day, split into 2°
	// ranges. The chance of each range comes from two places, joined: what the station has already
	// recorded today (fact), and what 82 forecast runs say about the hours still to come (a spread of
	// possible days). Late in the day the record decides almost everything: that is the "known" side.
	// The day before, the forecasts do: that is the forecast side. Same model, different settings.
	KIND_WEATHER       = "weather"
	WEATHER_NAME       = "Weather"
	WEATHER_ABOUT      = "Daily high and low temperatures in 24 US cities: today's station record joined with 82 forecast runs into a chance for every 2° range."
	KNOWN_ID           = "weather-known"
	KNOWN_NAME         = "Weather: what's already recorded"
	FORECAST_ID        = "weather-forecast"
	FORECAST_NAME      = "Weather: forecast edge"
	STRIKE_GREATER     = "greater"
	STRIKE_LESS        = "less"
	STRIKE_BETWEEN     = "between"
	HALF_DEGREE        = 0.5 // settled values are whole degrees: "64° to 65°" is 63.5 up to 65.5
	EVENT_DATE_LAYOUT  = "06Jan02"
	MIN_RUNS           = 10 // fewer usable forecast runs than this is no forecast
	STD_MONTH          = time.January
	STD_DAY            = 15
	LEAD_DAYS_MAX      = 3.0
	DAY                = 24 * time.Hour
	WEATHER_REASON_P   = "model %.0f%% YES from %d runs"
	WEATHER_REASON_OBS = "%s so far %.1f°"
	WEATHER_REASON_SD  = "±%.1f°"
	WEATHER_REASON_FIN = "the day is over: %.1f° recorded"
	WORD_HIGH          = "high"
	WORD_LOW           = "low"

	DEFAULT_SIGMA_F       = 0.8 // the least spread around a forecast run: rounding, the settling source, peaks between hours
	DEFAULT_SIGMA_DAY_F   = 1.2 // more per day still to come: forecast runs agree more than the weather does
	DEFAULT_HOURLY_PEAK_F = 0.5 // hourly forecast values miss the day's true peak by about this much
	DEFAULT_RECORD_SD_F   = 0.4 // the settled value against the station's own readings: rounding, the settling source
	DEFAULT_RECORD_PEAK_F = 0.3 // readings miss the true peak between them by about this much
	DEFAULT_KNOWN_MARGIN  = 1.0 // known: the record must clear a range's edge by this much
)

//go:embed weather_stations.json
var defaultStations []byte

var nan = math.NaN()

// Station is one weather station Kalshi markets settle on, and the series that settle on it.
type Station struct {
	Station string   `json:"station"` // the NWS station id (KNYC)
	City    string   `json:"city"`
	Lat     float64  `json:"lat"`
	Lon     float64  `json:"lon"`
	TZ      string   `json:"tz"`
	High    []string `json:"high"` // series tickers of daily-high markets
	Low     []string `json:"low"`
}

// WeatherParams are the weather kind's own settings.
type WeatherParams struct {
	StationsFile string   `json:"stations_file,omitempty"` // a JSON list of stations; empty: the built-in 24
	Only         []string `json:"only,omitempty"`          // station ids to trade; empty: all
	Highs        bool     `json:"highs"`
	Lows         bool     `json:"lows"`
	DaysAhead    int      `json:"days_ahead"`      // 0: today only; 1: today and tomorrow
	SigmaF       float64  `json:"sigma_f"`         // the least spread put around every run, °F
	SigmaPerDayF float64  `json:"sigma_per_day_f"` // added per day of forecast still to come
	HourlyPeakF  float64  `json:"hourly_peak_f"`
	RecordSigmaF float64  `json:"record_sigma_f"` // the spread around what was already recorded
	RecordPeakF  float64  `json:"record_peak_f"`
	// KnownOnly takes only markets the record has already decided: a high already above a range, a low
	// already below one, or a finished day well inside one. Nothing the forecast still has to get right.
	KnownOnly    bool    `json:"known_only"`
	KnownMarginF float64 `json:"known_margin_f"`
}

type seriesInfo struct {
	station Station
	high    bool
	loc     *time.Location // standard time all year: the climate day runs midnight to midnight standard time
}

type weather struct {
	p      WeatherParams
	series map[string]seriesInfo
	data   *weatherData
	env    *Env
}

func init() {
	Register(Kind{ID: KIND_WEATHER, Name: WEATHER_NAME, About: WEATHER_ABOUT, New: newWeather})
}

// sharedWeather is one cache for every weather strategy, so two of them never fetch the same forecast twice.
var sharedWeather = &weatherData{ensembles: map[string]*Ensemble{}, readings: map[string]*Readings{}}

func newWeather(raw json.RawMessage) (Strategy, error) {
	p := WeatherParams{Highs: true, Lows: true, SigmaF: DEFAULT_SIGMA_F, SigmaPerDayF: DEFAULT_SIGMA_DAY_F, HourlyPeakF: DEFAULT_HOURLY_PEAK_F,
		RecordSigmaF: DEFAULT_RECORD_SD_F, RecordPeakF: DEFAULT_RECORD_PEAK_F, KnownMarginF: DEFAULT_KNOWN_MARGIN}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	if p.SigmaF <= 0 || p.RecordSigmaF <= 0 || p.DaysAhead < 0 {
		return nil, errors.New("sigma_f and record_sigma_f must be above 0, days_ahead not negative")
	}
	if p.KnownMarginF < 0 {
		return nil, errors.New("known_margin_f must not be negative")
	}
	body := defaultStations
	if p.StationsFile != "" {
		b, err := os.ReadFile(p.StationsFile)
		if err != nil {
			return nil, err
		}
		body = b
	}
	var stations []Station
	if err := json.Unmarshal(body, &stations); err != nil {
		return nil, fmt.Errorf("stations: %w", err)
	}
	only := map[string]bool{}
	for _, id := range p.Only {
		only[strings.ToUpper(id)] = true
	}
	w := &weather{p: p, series: map[string]seriesInfo{}, data: sharedWeather}
	for _, s := range stations {
		if len(only) > 0 && !only[s.Station] {
			continue
		}
		loc, err := standardZone(s.TZ)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.Station, err)
		}
		if p.Highs {
			for _, t := range s.High {
				w.series[t] = seriesInfo{station: s, high: true, loc: loc}
			}
		}
		if p.Lows {
			for _, t := range s.Low {
				w.series[t] = seriesInfo{station: s, high: false, loc: loc}
			}
		}
	}
	return w, nil
}

// standardZone is a fixed zone at the place's standard offset, the clock climate days are kept in.
func standardZone(name string) (*time.Location, error) {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	_, offset := time.Date(time.Now().Year(), STD_MONTH, STD_DAY, 12, 0, 0, 0, loc).Zone()
	return time.FixedZone(name, offset), nil
}

func (w *weather) Scope() Scope {
	out := make([]string, 0, len(w.series))
	for t := range w.series {
		out = append(out, t)
	}
	return Scope{Series: out}
}

func (w *weather) Prepare(ctx context.Context, env *Env, markets []Market, now time.Time) error {
	w.env = env
	needed := map[string]Station{}
	for _, m := range markets {
		if info, ok := w.series[m.Series()]; ok {
			needed[info.station.Station] = info.station
		}
	}
	var errs []error
	for _, s := range needed {
		if _, err := w.data.loadEnsemble(ctx, env, s, now); err != nil {
			errs = append(errs, err)
		}
		if _, err := w.data.loadReadings(ctx, env, s, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// window is the climate day a market is about, in UTC.
func (w *weather) window(info seriesInfo, m Market) (time.Time, time.Time, bool) {
	day, err := m.EventDate()
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, info.loc)
	return start.UTC(), start.Add(DAY).UTC(), true
}

func (w *weather) Evaluate(m Market, now time.Time) (Opinion, bool) {
	info, ok := w.series[m.Series()]
	lo, hi, okRange := m.Range()
	if !ok || !okRange {
		return Opinion{}, false
	}
	start, end, ok := w.window(info, m)
	local := now.In(info.loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, info.loc).UTC()
	if !ok || start.After(today.Add(time.Duration(w.p.DaysAhead)*DAY)) {
		return Opinion{}, false
	}
	w.data.mu.Lock()
	ens, obs := w.data.ensembles[info.station.Station], w.data.readings[info.station.Station]
	w.data.mu.Unlock()
	model, reasons, ok := w.model(info, ens, obs, start, end, now)
	if !ok || (w.p.KnownOnly && !model.decided(lo, hi, w.p.KnownMarginF)) {
		return Opinion{}, false
	}
	prob := model.prob(lo, hi)
	side := SIDE_YES
	if (DOLLAR-prob)-m.NoAsk > prob-m.YesAsk {
		side = SIDE_NO
	}
	reasons = append([]string{fmt.Sprintf(WEATHER_REASON_P, prob*PERCENT, max(1, len(model.runs)))}, reasons...)
	return Opinion{Side: side, Prob: prob, Reasons: reasons}, true
}

// dayModel is what is known about a day's high (or low): the value recorded so far, which the final
// one can only pass, and each forecast run's value for the hours still to come.
type dayModel struct {
	high        bool
	record      float64 // recorded so far, moved by the peak the readings miss
	recorded    float64 // recorded so far, as read
	hasRecord   bool
	over        bool // the day has ended: the record is the whole day
	recordSigma float64
	runs        []float64
	runSigma    float64
}

// below is the chance the day's value ends under x. A high is the larger of the record and the rest of
// the day; a low the smaller. The record and each run carry a normal spread of their own.
func (d dayModel) below(x float64) float64 {
	rec := 1.0 // the chance the record alone stays under x
	if d.hasRecord {
		rec = normalCDF((x - d.record) / d.recordSigma)
	} else if !d.high {
		rec = 0
	}
	if len(d.runs) == 0 {
		return rec
	}
	total := 0.0
	for _, v := range d.runs {
		run := normalCDF((x - v) / d.runSigma)
		if d.high {
			total += rec * run
			continue
		}
		total += DOLLAR - (DOLLAR-rec)*(DOLLAR-run)
	}
	return total / float64(len(d.runs))
}

// prob is the chance the day's value lands in [lo, hi).
func (d dayModel) prob(lo, hi float64) float64 { return math.Max(0, d.below(hi)-d.below(lo)) }

// decided says whether the record alone settles the range [lo, hi), with margin degrees to spare. A
// high can only rise from here and a low only fall, so a range the record has passed is out.
func (d dayModel) decided(lo, hi, margin float64) bool {
	r := d.recorded
	below := r < lo && r <= lo-margin
	switch {
	case !d.hasRecord:
		return false
	case d.over:
		return below || r >= hi+margin || (r >= lo+margin && r < hi && r <= hi-margin)
	case d.high:
		return r >= hi+margin || (math.IsInf(hi, 1) && r >= lo+margin)
	}
	return below || (math.IsInf(lo, -1) && r < hi && r <= hi-margin)
}

// model joins what the station recorded today with the forecast runs for the hours left.
func (w *weather) model(info seriesInfo, ens *Ensemble, obs *Readings, start, end, now time.Time) (dayModel, []string, bool) {
	word, peak, recordPeak := WORD_LOW, -w.p.HourlyPeakF, -w.p.RecordPeakF
	if info.high {
		word, peak, recordPeak = WORD_HIGH, w.p.HourlyPeakF, w.p.RecordPeakF
	}
	d := dayModel{high: info.high, recordSigma: w.p.RecordSigmaF}
	recorded := 0.0
	if obs != nil && now.After(start) {
		recorded, d.hasRecord = obs.Extreme(start, minTime(now, end), info.high)
		d.record, d.recorded = recorded+recordPeak, recorded
	}
	if !now.Before(end) {
		if !d.hasRecord || obs.At.Before(end) {
			return d, nil, false
		}
		d.over = true
		return d, []string{fmt.Sprintf(WEATHER_REASON_FIN, recorded)}, true
	}
	if ens == nil {
		return d, nil, false
	}
	from := maxTime(start, now.Truncate(time.Hour))
	for _, run := range ens.Members {
		if v, found := runExtreme(ens.Hours, run, from, end, info.high); found {
			d.runs = append(d.runs, v+peak)
		}
	}
	if len(d.runs) < MIN_RUNS {
		return d, nil, false
	}
	lead := math.Min(LEAD_DAYS_MAX, math.Max(0, end.Sub(maxTime(now, start)).Hours()/24))
	d.runSigma = w.p.SigmaF + w.p.SigmaPerDayF*lead
	reasons := []string{fmt.Sprintf(WEATHER_REASON_SD, d.runSigma)}
	if d.hasRecord {
		reasons = append(reasons, fmt.Sprintf(WEATHER_REASON_OBS, word, recorded))
	}
	return d, reasons, true
}

// runExtreme is one run's highest (or lowest) hourly value from from up to to.
func runExtreme(hours []time.Time, run []float64, from, to time.Time, high bool) (float64, bool) {
	best, found := 0.0, false
	for i, h := range hours {
		if h.Before(from) || !h.Before(to) || math.IsNaN(run[i]) {
			continue
		}
		if !found || (high && run[i] > best) || (!high && run[i] < best) {
			best, found = run[i], true
		}
	}
	return best, found
}

func normalCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func weatherParams(daysAhead int, knownOnly bool) json.RawMessage {
	raw, _ := json.Marshal(WeatherParams{Highs: true, Lows: true, DaysAhead: daysAhead, SigmaF: DEFAULT_SIGMA_F, SigmaPerDayF: DEFAULT_SIGMA_DAY_F,
		HourlyPeakF: DEFAULT_HOURLY_PEAK_F, RecordSigmaF: DEFAULT_RECORD_SD_F, RecordPeakF: DEFAULT_RECORD_PEAK_F, KnownOnly: knownOnly, KnownMarginF: DEFAULT_KNOWN_MARGIN})
	return raw
}

func defaultWeatherKnown() Config {
	return Config{ID: KNOWN_ID, Kind: KIND_WEATHER, Name: KNOWN_NAME, On: true, Risk: RISK_LOW, EveryMinutes: DEFAULT_EVERY_MINUTES,
		Entry:  Entry{MinPrice: 0.50, MaxPrice: 0.97, MaxSpread: 0.05, LeftMinutes: DEFAULT_LEFT_MINUTES, MinEdge: 0.03, MinProb: 0.95},
		Limits: Limits{StakeUSD: 15, MaxOpen: 15, NewPerPass: 5, DailyStakeUSD: 150}, Params: weatherParams(0, true)}
}

func defaultWeatherForecast() Config {
	return Config{ID: FORECAST_ID, Kind: KIND_WEATHER, Name: FORECAST_NAME, On: true, Risk: RISK_MEDIUM, EveryMinutes: DEFAULT_EVERY_MINUTES * 2,
		Entry:  Entry{MinPrice: 0.08, MaxPrice: 0.92, MaxSpread: 0.06, MinVolume24h: 50, LeftMinutes: 30, MinEdge: 0.08, MaxEdge: 0.25},
		Limits: Limits{StakeUSD: 10, MaxOpen: 20, NewPerPass: 4, DailyStakeUSD: 100}, Params: weatherParams(1, false)}
}
