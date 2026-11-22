package predict

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

type observationPages struct{ pages int }

func (o *observationPages) RoundTrip(r *http.Request) (*http.Response, error) {
	o.pages++
	body := `{"features":[{"properties":{"timestamp":"2026-11-22T18:00:00Z","temperature":{"value":14}}}],"pagination":{"next":"https://api.weather.gov/stations/KNYC/observations?cursor=older"}}`
	if r.URL.Query().Get("cursor") == "older" {
		body = `{"features":[{"properties":{"timestamp":"2026-11-22T12:00:00Z","temperature":{"value":20}}}]}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestReadingsIncludeOlderObservationPages(t *testing.T) {
	client := &observationPages{}
	env := &Env{HTTP: &http.Client{Transport: client}}
	data := &weatherData{readings: map[string]*Readings{}}
	start := time.Date(2026, 11, 22, 5, 0, 0, 0, time.UTC)
	now := start.Add(16 * time.Hour)
	obs, err := data.loadReadings(t.Context(), env, Station{Station: "KNYC"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := obs.Extreme(start, now, true); !ok || got != 68 || client.pages != 2 {
		t.Fatalf("got high %v over %d response pages, want 68 over two pages", got, client.pages)
	}
}

func TestWeatherDoesNotTreatPreMidnightCacheAsCompletedDay(t *testing.T) {
	s, err := newWeather(nil)
	if err != nil {
		t.Fatal(err)
	}
	w := s.(*weather)
	info := w.series["KXHIGHNY"]
	start := time.Date(2026, 7, 4, 5, 0, 0, 0, time.UTC)
	end := start.Add(DAY)
	m := Market{Event: "KXHIGHNY-26JUL04"}
	if from, to, ok := w.window(info, m); !ok || !from.Equal(start) || !to.Equal(end) {
		t.Fatalf("summer climate day %v to %v, want %v to %v", from, to, start, end)
	}
	obs := &Readings{At: end.Add(-time.Minute), Times: []time.Time{end.Add(-time.Hour)}, TempsF: []float64{75}}
	now := end.Add(2 * time.Minute)
	if _, _, ok := w.model(info, nil, obs, start, end, now); ok {
		t.Error("pre-midnight observations treated as a completed day")
	}
	obs.At = now
	if _, _, ok := w.model(info, nil, obs, start, end, now); !ok {
		t.Error("observations fetched after midnight did not produce a model")
	}
}

func TestKnownWeatherRespectsHalfOpenStrikeAtZeroMargin(t *testing.T) {
	lo, hi := 63.5, 65.5
	low := dayModel{hasRecord: true, recorded: lo}
	if low.decided(lo, hi, 0) {
		t.Error("a low exactly at the inclusive lower bound can still fall out of the range")
	}
	high := dayModel{hasRecord: true, high: true, recorded: hi}
	if !high.decided(lo, hi, 0) || low.decided(math.Inf(-1), lo, 0) {
		t.Error("an observation at the exclusive upper bound is outside the range")
	}
	if _, err := newWeather(json.RawMessage(`{"known_margin_f":-1}`)); err == nil {
		t.Error("a negative margin can declare a still-open range decided")
	}
}

func TestWeatherDoesNotInventStrikeWhenKalshiOmitsBound(t *testing.T) {
	var wire wireMarket
	if err := json.Unmarshal([]byte(`{"strike_type":"greater","floor_strike":null}`), &wire); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := wire.market().Range(); ok {
		t.Error("missing strike was interpreted as 0°F")
	}
	if err := json.Unmarshal([]byte(`{"strike_type":"greater","floor_strike":0}`), &wire); err != nil {
		t.Fatal(err)
	}
	if lo, _, ok := wire.market().Range(); !ok || lo != 0.5 {
		t.Error("explicit 0°F strike was rejected")
	}
}
