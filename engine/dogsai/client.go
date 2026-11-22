// Package dogsai is a client for the finance worker in the DOGS AI app: headline sentiment (FinBERT) and
// price forecasts (Kronos, Chronos, TimesFM). DOGS AI is a separate app with its own tray icon; Money 24
// only calls it. When it is not running, or runs without the finance worker, every call says so with
// ErrUnavailable, and callers carry on without it.
//
// What it answers is information, like any model's read: it is shown and recorded, never traded on.
package dogsai

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	ENV_URL        = "M24D_DOGS_AI" // a finance worker somewhere else; wins over discovery
	ENV_DATA       = "DOGS_AI_DATA" // DOGS AI's own override for its data folder
	ENV_PORT       = "DOGS_PORT"    // DOGS AI's own override for its port
	ENV_LOCAL      = "LOCALAPPDATA"
	ENV_XDG        = "XDG_DATA_HOME"
	DATA_NAME      = "DOGS AI" // the data folder on Windows and macOS
	DATA_NAME_UNIX = "dogs-ai" // the data folder on Linux
	SERVER_FILE    = "server.json"
	DEFAULT_URL    = "http://127.0.0.1:8788"
	LOOPBACK       = "http://127.0.0.1:"
	OS_WINDOWS     = "windows"
	OS_MAC         = "darwin"

	MODELS_PATH    = "/api/finance/models"
	SENTIMENT_PATH = "/api/finance/sentiment"
	FORECAST_PATH  = "/api/finance/forecast"

	PROBE_TIMEOUT     = 2 * time.Second
	CALL_TIMEOUT      = 2 * time.Minute // a forecast model loading for the first time
	MAX_BODY          = 4 << 20
	MAX_ERROR         = 300
	MAX_TEXTS         = 64   // headlines per sentiment call; longer lists are sent in batches
	MAX_TEXT_CHARS    = 1000 // FinBERT reads 512 tokens; a headline is far shorter
	MAX_SERIES        = 2048 // bars per forecast
	MAX_HORIZON       = 256
	MS_THRESHOLD      = 1e11 // a Unix time above this is in milliseconds
	SENTIMENT_POS     = "positive"
	SENTIMENT_NEG     = "negative"
	SENTIMENT_NEUTRAL = "neutral"
	MODEL_KRONOS      = "kronos"
	MODEL_CHRONOS     = "chronos"
	MODEL_TIMESFM     = "timesfm"
)

var (
	ErrUnavailable = errors.New("the DOGS AI finance worker is not running")
	ErrBadInput    = errors.New("bad input for the finance worker")
	ErrBadAnswer   = errors.New("the finance worker answered in the wrong shape")
	FORECASTERS    = []string{MODEL_KRONOS, MODEL_CHRONOS, MODEL_TIMESFM}
	SENTIMENTS     = []string{SENTIMENT_POS, SENTIMENT_NEG, SENTIMENT_NEUTRAL}
)

// Client is one DOGS AI engine.
type Client struct {
	Base string
	Key  string // empty unless the engine takes one (served to the network)
	HTTP *http.Client
}

// dataDir is DOGS AI's shared data folder, by the same rules DOGS AI uses.
func dataDir() string {
	if dir := os.Getenv(ENV_DATA); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case OS_WINDOWS:
		return filepath.Join(cmp.Or(os.Getenv(ENV_LOCAL), filepath.Join(home, "AppData", "Local")), DATA_NAME)
	case OS_MAC:
		return filepath.Join(home, "Library", "Application Support", DATA_NAME)
	}
	return filepath.Join(cmp.Or(os.Getenv(ENV_XDG), filepath.Join(home, ".local", "share")), DATA_NAME_UNIX)
}

// Discover finds the engine: M24D_DOGS_AI, else the one recorded in server.json in DOGS AI's data
// folder (with its key), else DOGS_PORT on loopback, else the default port. Finding an address does not
// mean anything answers there; Models says that.
func Discover(client *http.Client) Client {
	c := Client{Base: DEFAULT_URL, HTTP: cmp.Or(client, &http.Client{})}
	if u := strings.TrimSpace(os.Getenv(ENV_URL)); u != "" {
		c.Base = strings.TrimRight(u, "/")
		return c
	}
	if raw, err := os.ReadFile(filepath.Join(dataDir(), SERVER_FILE)); err == nil {
		var info struct {
			URL  string `json:"url"`
			Port int    `json:"port"`
			Key  string `json:"key"`
		}
		if json.Unmarshal(raw, &info) == nil && (info.URL != "" || info.Port > 0) {
			c.Base = strings.TrimRight(cmp.Or(info.URL, LOOPBACK+strconv.Itoa(info.Port)), "/")
			c.Key = info.Key
			return c
		}
	}
	if port := os.Getenv(ENV_PORT); port != "" {
		c.Base = LOOPBACK + port
	}
	return c
}

// Model is one model the finance worker serves.
type Model struct {
	ID    string `json:"id"`
	Task  string `json:"task"` // sentiment or forecast
	Ready bool   `json:"ready"`
}

// Models lists what the worker serves. It is also the check that the worker is there at all. The
// contract does not fix the list's shape, so a bare array, {"models":[...]} and plain names all read.
func (c Client) Models(ctx context.Context) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, PROBE_TIMEOUT)
	defer cancel()
	var raw json.RawMessage
	if err := c.call(ctx, http.MethodGet, MODELS_PATH, nil, &raw); err != nil {
		return nil, err
	}
	var wrapped struct {
		Models json.RawMessage `json:"models"`
	}
	if json.Unmarshal(raw, &wrapped) == nil && wrapped.Models != nil {
		raw = wrapped.Models
	}
	var models []Model
	if json.Unmarshal(raw, &models) == nil {
		return models, nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadAnswer, err)
	}
	for _, n := range names {
		models = append(models, Model{ID: n, Ready: true})
	}
	return models, nil
}

// Sentiment is FinBERT's read of one headline.
type Sentiment struct {
	Label string  `json:"label"` // positive, negative or neutral
	Score float64 `json:"score"` // the model's confidence in the label, 0 to 1
}

// SentimentResult is one sentiment answer, in the order the texts were sent.
type SentimentResult struct {
	Model   string      `json:"model"`
	Results []Sentiment `json:"results"`
}

// Sentiment reads headlines in batches of MAX_TEXTS. Every text gets exactly one result, or the call fails.
func (c Client) Sentiment(ctx context.Context, texts []string) (SentimentResult, error) {
	if len(texts) == 0 {
		return SentimentResult{}, fmt.Errorf("%w: no texts", ErrBadInput)
	}
	out := SentimentResult{Results: make([]Sentiment, 0, len(texts))}
	for start := 0; start < len(texts); start += MAX_TEXTS {
		batch := texts[start:min(start+MAX_TEXTS, len(texts))]
		clipped := make([]string, len(batch))
		for i, t := range batch {
			clipped[i] = t[:min(len(t), MAX_TEXT_CHARS)]
		}
		var res SentimentResult
		if err := c.callTimed(ctx, SENTIMENT_PATH, map[string]any{"texts": clipped}, &res); err != nil {
			return SentimentResult{}, err
		}
		if len(res.Results) != len(batch) {
			return SentimentResult{}, fmt.Errorf("%w: %d results for %d texts", ErrBadAnswer, len(res.Results), len(batch))
		}
		for i, r := range res.Results {
			r.Label = strings.ToLower(r.Label)
			if !slices.Contains(SENTIMENTS, r.Label) {
				return SentimentResult{}, fmt.Errorf("%w: label %q", ErrBadAnswer, r.Label)
			}
			res.Results[i] = r
		}
		out.Model = cmp.Or(out.Model, res.Model)
		out.Results = append(out.Results, res.Results...)
	}
	return out, nil
}

// Time is a moment the worker may write as Unix seconds, Unix milliseconds or RFC 3339. It is sent as
// Unix seconds.
type Time struct{ time.Time }

func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(t.Unix(), 10)), nil
}

func (t *Time) UnmarshalJSON(b []byte) error {
	var n float64
	if err := json.Unmarshal(b, &n); err == nil {
		if n > MS_THRESHOLD {
			t.Time = time.UnixMilli(int64(n)).UTC()
			return nil
		}
		t.Time = time.Unix(int64(n), 0).UTC()
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, s)
	t.Time = parsed.UTC()
	return err
}

// Bar is one candle sent to a forecaster.
type Bar struct {
	T      Time    `json:"t"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// Point is one forecast step: the expected close and its band.
type Point struct {
	T     Time    `json:"t"`
	Close float64 `json:"close"`
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
}

// ForecastResult is one forecast.
type ForecastResult struct {
	Model    string  `json:"model"`
	Forecast []Point `json:"forecast"`
}

// Forecast asks one forecaster for the next horizon bars after the series.
func (c Client) Forecast(ctx context.Context, model string, series []Bar, horizon int) (ForecastResult, error) {
	if !slices.Contains(FORECASTERS, model) {
		return ForecastResult{}, fmt.Errorf("%w: model %q is not one of %v", ErrBadInput, model, FORECASTERS)
	}
	if len(series) == 0 || len(series) > MAX_SERIES || horizon < 1 || horizon > MAX_HORIZON {
		return ForecastResult{}, fmt.Errorf("%w: %d bars, horizon %d", ErrBadInput, len(series), horizon)
	}
	var res ForecastResult
	body := map[string]any{"model": model, "series": series, "horizon": horizon}
	if err := c.callTimed(ctx, FORECAST_PATH, body, &res); err != nil {
		return ForecastResult{}, err
	}
	if len(res.Forecast) == 0 || len(res.Forecast) > horizon {
		return ForecastResult{}, fmt.Errorf("%w: %d points for horizon %d", ErrBadAnswer, len(res.Forecast), horizon)
	}
	for _, p := range res.Forecast {
		if p.Low > p.Close || p.Close > p.High {
			return ForecastResult{}, fmt.Errorf("%w: band %.4f..%.4f does not hold %.4f", ErrBadAnswer, p.Low, p.High, p.Close)
		}
	}
	return res, nil
}

// callTimed is a POST under CALL_TIMEOUT unless the caller's deadline is sooner.
func (c Client) callTimed(ctx context.Context, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, CALL_TIMEOUT)
	defer cancel()
	return c.call(ctx, http.MethodPost, path, body, out)
}

// call sends one request. No answer, or an engine without the finance routes (404), is ErrUnavailable.
func (c Client) call(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w at %s: %v", ErrUnavailable, c.Base, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, MAX_BODY))
	if err != nil {
		return err
	}
	msg := strings.Join(strings.Fields(string(data)), " ")
	msg = msg[:min(len(msg), MAX_ERROR)]
	if res.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: DOGS AI at %s has no %s (%s)", ErrUnavailable, c.Base, path, msg)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("DOGS AI HTTP %d: %s", res.StatusCode, msg)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: %v", ErrBadAnswer, err)
	}
	return nil
}
