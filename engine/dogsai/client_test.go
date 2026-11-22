package dogsai

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const TEST_KEY = "k123"

// fixtureServer answers the finance routes with the files in testdata, and checks the key when one is set.
func fixtureServer(t *testing.T, key string) *httptest.Server {
	t.Helper()
	routes := map[string]string{MODELS_PATH: "models.json", SENTIMENT_PATH: "sentiment.json", FORECAST_PATH: "forecast.json"}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key != "" && r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		file, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"no route"}`))
			return
		}
		if r.URL.Path == FORECAST_PATH {
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Model  string            `json:"model"`
				Series []json.RawMessage `json:"series"`
			}
			json.Unmarshal(body, &req)
			if req.Model != MODEL_KRONOS || len(req.Series) != 2 || !strings.Contains(string(req.Series[0]), `"t":1795330800`) {
				t.Errorf("forecast request wrong: %s", body)
			}
		}
		data, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}))
}

// Discovery reads server.json from DOGS AI's data folder, key included; M24D_DOGS_AI wins over it.
func TestDiscoverReadsServerJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ENV_DATA, dir)
	t.Setenv(ENV_URL, "")
	t.Setenv(ENV_PORT, "")
	if c := Discover(nil); c.Base != DEFAULT_URL {
		t.Fatalf("no server.json: want the default, got %s", c.Base)
	}
	t.Setenv(ENV_PORT, "8790")
	if c := Discover(nil); c.Base != "http://127.0.0.1:8790" {
		t.Fatalf("DOGS_PORT: got %s", c.Base)
	}
	os.WriteFile(filepath.Join(dir, SERVER_FILE), []byte(`{"url":"http://127.0.0.1:8791/","port":8791,"keyed":true,"key":"k123"}`), 0o600)
	if c := Discover(nil); c.Base != "http://127.0.0.1:8791" || c.Key != TEST_KEY {
		t.Fatalf("server.json: got %+v", c)
	}
	os.WriteFile(filepath.Join(dir, SERVER_FILE), []byte(`{"port":8792}`), 0o600)
	if c := Discover(nil); c.Base != "http://127.0.0.1:8792" {
		t.Fatalf("port only: got %s", c.Base)
	}
	t.Setenv(ENV_URL, "http://box:9000/")
	if c := Discover(nil); c.Base != "http://box:9000" || c.Key != "" {
		t.Fatalf("M24D_DOGS_AI: got %+v", c)
	}
}

func TestModelsSentimentForecast(t *testing.T) {
	srv := fixtureServer(t, TEST_KEY)
	defer srv.Close()
	c := Client{Base: srv.URL, Key: TEST_KEY, HTTP: srv.Client()}
	models, err := c.Models(t.Context())
	if err != nil || len(models) != 4 || models[2].Ready {
		t.Fatalf("models: %v %+v", err, models)
	}
	sent, err := c.Sentiment(t.Context(), []string{"Nvidia probed by SEC", "Record debut", "Board meets Tuesday"})
	if err != nil || sent.Results[0].Label != SENTIMENT_NEG || sent.Results[1].Label != SENTIMENT_POS {
		t.Fatalf("sentiment: %v %+v", err, sent)
	}
	from := time.Unix(1795330800, 0)
	series := []Bar{{T: Time{from}, Open: 125, High: 126, Low: 124, Close: 125.5, Volume: 1e6},
		{T: Time{from.Add(time.Hour)}, Open: 125.5, High: 127, Low: 125, Close: 126, Volume: 9e5}}
	fc, err := c.Forecast(t.Context(), MODEL_KRONOS, series, 3)
	if err != nil || len(fc.Forecast) != 3 {
		t.Fatalf("forecast: %v %+v", err, fc)
	}
	// Seconds, an RFC 3339 string and milliseconds all read as the same kind of moment.
	want := []int64{1795339200, 1795446000, 1795346400}
	for i, p := range fc.Forecast {
		if p.T.Unix() != want[i] {
			t.Errorf("point %d: t=%d want %d", i, p.T.Unix(), want[i])
		}
	}
}

// A sentiment answer with the wrong number of results is refused rather than misaligned with its headlines.
func TestSentimentCountMismatch(t *testing.T) {
	srv := fixtureServer(t, "")
	defer srv.Close()
	c := Client{Base: srv.URL, HTTP: srv.Client()}
	if _, err := c.Sentiment(t.Context(), []string{"only one"}); !errors.Is(err, ErrBadAnswer) {
		t.Fatalf("want ErrBadAnswer, got %v", err)
	}
	if _, err := c.Forecast(t.Context(), "prophet", []Bar{{}}, 3); !errors.Is(err, ErrBadInput) {
		t.Fatalf("want ErrBadInput for an unknown model, got %v", err)
	}
}

// Not running, or running without the finance worker: both are ErrUnavailable, quickly.
func TestUnavailable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	down := Client{Base: "http://" + addr, HTTP: http.DefaultClient}
	if _, err := down.Models(t.Context()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("down: want ErrUnavailable, got %v", err)
	}
	if _, err := down.Sentiment(t.Context(), []string{"x"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("down sentiment: want ErrUnavailable, got %v", err)
	}
	older := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"no route GET /api/finance/models"}`))
	}))
	defer older.Close()
	if _, err := (Client{Base: older.URL, HTTP: older.Client()}).Models(t.Context()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no finance routes: want ErrUnavailable, got %v", err)
	}
}
