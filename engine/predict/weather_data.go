package predict

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// Forecasts: the US (GEFS, 31 runs) and European (ECMWF, 51 runs) ensembles through Open-Meteo,
	// free for non-commercial use, hourly, in °F. Each run is one possible day; together they are a
	// probability. They update four times a day, so an hour's cache misses nothing.
	ENSEMBLE_URL    = "https://ensemble-api.open-meteo.com/v1/ensemble"
	ENSEMBLE_MODELS = "gfs025,ecmwf_ifs025"
	ENSEMBLE_VAR    = "temperature_2m"
	ENSEMBLE_EVERY  = time.Hour
	ENSEMBLE_LAYOUT = "2006-01-02T15:04"
	ENSEMBLE_PAST   = "1" // days back, so today's first hours are in the series
	ENSEMBLE_AHEAD  = "3" // days ahead

	// Observations: the station's own readings from the National Weather Service, in °C to a tenth.
	OBS_URL       = "https://api.weather.gov/stations/%s/observations"
	OBS_EVERY     = 10 * time.Minute
	OBS_BACK      = 36 * time.Hour // yesterday's whole climate day too: its markets trade past midnight
	OBS_MAX_PAGES = 12
	OBS_CURSOR    = "cursor"
	OBS_ACCEPT    = "application/geo+json"
	BODY_MAX      = 16 << 20
	F_PER_C       = 9.0 / 5.0
	F_FREEZE      = 32.0
	DATA_TIMEOUT  = 30 * time.Second
)

// Env is what strategies share to reach outside data.
type Env struct {
	HTTP *http.Client
}

func (e *Env) getJSON(ctx context.Context, u string, header map[string]string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, DATA_TIMEOUT)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", KALSHI_USER_AGENT)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := e.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, BODY_MAX))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", req.URL.Host, res.Status)
	}
	return json.Unmarshal(body, out)
}

// Ensemble is every run's hourly temperature at one place, °F, by UTC hour.
type Ensemble struct {
	At      time.Time   // when it was fetched
	Hours   []time.Time // UTC
	Members [][]float64 // members × hours; NaN where a run has no value
}

// Readings are a station's observations, oldest first.
type Readings struct {
	At     time.Time
	Times  []time.Time
	TempsF []float64
}

// weatherData caches forecasts and readings per station between passes.
type weatherData struct {
	mu        sync.Mutex
	ensembles map[string]*Ensemble
	readings  map[string]*Readings
}

func (w *weatherData) loadEnsemble(ctx context.Context, env *Env, s Station, now time.Time) (*Ensemble, error) {
	w.mu.Lock()
	cached := w.ensembles[s.Station]
	w.mu.Unlock()
	if cached != nil && now.Sub(cached.At) < ENSEMBLE_EVERY {
		return cached, nil
	}
	q := url.Values{"latitude": {strconv.FormatFloat(s.Lat, 'f', -1, 64)}, "longitude": {strconv.FormatFloat(s.Lon, 'f', -1, 64)},
		"hourly": {ENSEMBLE_VAR}, "models": {ENSEMBLE_MODELS}, "temperature_unit": {"fahrenheit"}, "timezone": {"GMT"},
		"past_days": {ENSEMBLE_PAST}, "forecast_days": {ENSEMBLE_AHEAD}}
	var doc struct {
		Hourly map[string]json.RawMessage `json:"hourly"`
	}
	if err := env.getJSON(ctx, ENSEMBLE_URL+"?"+q.Encode(), nil, &doc); err != nil {
		return cached, err
	}
	var times []string
	if err := json.Unmarshal(doc.Hourly["time"], &times); err != nil {
		return cached, fmt.Errorf("ensemble times: %w", err)
	}
	e := &Ensemble{At: now}
	for _, t := range times {
		h, err := time.Parse(ENSEMBLE_LAYOUT, t)
		if err != nil {
			return cached, err
		}
		e.Hours = append(e.Hours, h.UTC())
	}
	for key, raw := range doc.Hourly {
		if !strings.HasPrefix(key, ENSEMBLE_VAR) {
			continue
		}
		var vals []*float64
		if json.Unmarshal(raw, &vals) != nil || len(vals) != len(e.Hours) {
			continue
		}
		row := make([]float64, len(vals))
		for i, v := range vals {
			row[i] = nan
			if v != nil {
				row[i] = *v
			}
		}
		e.Members = append(e.Members, row)
	}
	if len(e.Members) == 0 {
		return cached, fmt.Errorf("ensemble for %s: no runs", s.Station)
	}
	w.mu.Lock()
	w.ensembles[s.Station] = e
	w.mu.Unlock()
	return e, nil
}

func (w *weatherData) loadReadings(ctx context.Context, env *Env, s Station, now time.Time) (*Readings, error) {
	w.mu.Lock()
	cached := w.readings[s.Station]
	w.mu.Unlock()
	if cached != nil && now.Sub(cached.At) < OBS_EVERY {
		return cached, nil
	}
	type observationPage struct {
		Features []struct {
			Properties struct {
				Timestamp   string `json:"timestamp"`
				Temperature struct {
					Value *float64 `json:"value"`
				} `json:"temperature"`
			} `json:"properties"`
		} `json:"features"`
		Pagination struct {
			Next string `json:"next"`
		} `json:"pagination"`
	}
	base := fmt.Sprintf(OBS_URL, s.Station)
	query := url.Values{"start": {now.Add(-OBS_BACK).UTC().Format(time.RFC3339)}}
	r := &Readings{At: now}
	for page := 1; ; page++ {
		var doc observationPage
		if err := env.getJSON(ctx, base+"?"+query.Encode(), map[string]string{"Accept": OBS_ACCEPT}, &doc); err != nil {
			return cached, err
		}
		for _, feature := range doc.Features {
			p := feature.Properties
			t, err := time.Parse(time.RFC3339, p.Timestamp)
			if err != nil || p.Temperature.Value == nil {
				continue
			}
			r.Times = append(r.Times, t.UTC())
			r.TempsF = append(r.TempsF, *p.Temperature.Value*F_PER_C+F_FREEZE)
		}
		if doc.Pagination.Next == "" {
			break
		}
		if page >= OBS_MAX_PAGES {
			return cached, fmt.Errorf("observations for %s: more than %d pages", s.Station, OBS_MAX_PAGES)
		}
		next, err := url.Parse(doc.Pagination.Next)
		if err != nil {
			return cached, err
		}
		cursor := next.Query().Get(OBS_CURSOR)
		if cursor == "" || cursor == query.Get(OBS_CURSOR) {
			return cached, fmt.Errorf("observations for %s: invalid next cursor", s.Station)
		}
		query = url.Values{OBS_CURSOR: {cursor}}
	}
	slices.Reverse(r.Times)
	slices.Reverse(r.TempsF)
	w.mu.Lock()
	w.readings[s.Station] = r
	w.mu.Unlock()
	return r, nil
}

// Extreme is the highest (or lowest) reading between from and to, and whether there was one.
func (r *Readings) Extreme(from, to time.Time, high bool) (float64, bool) {
	best, found := 0.0, false
	for i, t := range r.Times {
		if t.Before(from) || !t.Before(to) {
			continue
		}
		v := r.TempsF[i]
		if !found || (high && v > best) || (!high && v < best) {
			best, found = v, true
		}
	}
	return best, found
}
