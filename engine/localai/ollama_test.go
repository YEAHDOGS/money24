package localai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const TEST_MODEL = "test-model:1b"

func TestHostNormalizesBindAddresses(t *testing.T) {
	cases := map[string]string{
		"":                       DEFAULT_OLLAMA_HOST,
		"0.0.0.0":                "http://127.0.0.1:11434",
		"0.0.0.0:11434":          "http://127.0.0.1:11434",
		":11500":                 "http://127.0.0.1:11500",
		"box.lan":                "http://box.lan:11434",
		"http://127.0.0.1:11434": "http://127.0.0.1:11434",
		"https://ai.example/":    "https://ai.example:11434",
		"[::]:11434":             "http://127.0.0.1:11434",
	}
	for in, want := range cases {
		if got := Host(in); got != want {
			t.Errorf("Host(%q) = %q, want %q", in, got, want)
		}
	}
}

// A fake Ollama checks the request carries the context window, memory mapping, thinking level and the
// schema twice (format field and prompt), and answers with timings that must come back in seconds.
func TestChatRequestAndTimings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != CHAT_PATH {
			t.Errorf("path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		opts, _ := req["options"].(map[string]any)
		if opts["num_ctx"] != float64(DEFAULT_CTX) || opts["use_mmap"] != true || req["think"] != "low" || req["stream"] != false {
			t.Errorf("options wrong: %v think=%v", opts, req["think"])
		}
		if _, ok := req["format"].(map[string]any); !ok {
			t.Error("schema missing from format")
		}
		msgs, _ := req["messages"].([]any)
		system, _ := msgs[0].(map[string]any)["content"].(string)
		if !strings.Contains(system, `"answer"`) {
			t.Error("schema missing from the system prompt")
		}
		json.NewEncoder(w).Encode(map[string]any{"model": TEST_MODEL, "message": map[string]any{"content": `{"answer":"x"}`},
			"done_reason": "stop", "prompt_eval_count": 50, "eval_count": 40, "load_duration": 2e9, "eval_duration": 4e9, "total_duration": 7e9})
	}))
	defer srv.Close()
	o := Ollama{Host: srv.URL, HTTP: srv.Client()}
	schema := map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}}}
	res, err := o.Chat(t.Context(), Request{Model: TEST_MODEL, System: "s", User: "u", Schema: schema, Think: "LOW"})
	if err != nil {
		t.Fatal(err)
	}
	if res.OutputTokens != 40 || res.LoadSeconds != 2 || res.TotalSeconds != 7 || res.TokensPerSecond() != 10 {
		t.Fatalf("timings wrong: %+v", res)
	}
}

// With nothing listening, every call says Ollama is down, quickly, and never hangs.
func TestDownIsErrDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	o := Ollama{Host: "http://" + addr, HTTP: http.DefaultClient}
	start := time.Now()
	if err := o.Up(t.Context()); !errors.Is(err, ErrDown) {
		t.Fatalf("want ErrDown, got %v", err)
	}
	if _, err := o.Chat(t.Context(), Request{Model: TEST_MODEL}); !errors.Is(err, ErrDown) {
		t.Fatalf("chat: want ErrDown, got %v", err)
	}
	if time.Since(start) > 2*PROBE_TIMEOUT {
		t.Errorf("down took %s to report", time.Since(start))
	}
}

// A server that accepts but never answers is cut off by the caller's deadline, not left hanging.
func TestSlowChatTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := Ollama{Host: srv.URL, HTTP: srv.Client()}.Chat(ctx, Request{Model: TEST_MODEL})
	if err == nil || !strings.Contains(err.Error(), "took too long") {
		t.Fatalf("want a timeout, got %v", err)
	}
}

func TestMissingModelIsErrNoModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model 'nope' not found"}`))
	}))
	defer srv.Close()
	_, err := Ollama{Host: srv.URL, HTTP: srv.Client()}.Chat(t.Context(), Request{Model: "nope"})
	if !errors.Is(err, ErrNoModel) {
		t.Fatalf("want ErrNoModel, got %v", err)
	}
}

func TestDecodeJSONFindsTheObject(t *testing.T) {
	var out struct{ A int }
	if err := DecodeJSON("Sure! ```json\n{\"A\": 3}\n```", &out); err != nil || out.A != 3 {
		t.Fatalf("fenced: %v %+v", err, out)
	}
	if err := DecodeJSON("no object here", &out); !errors.Is(err, ErrNoJSON) {
		t.Fatalf("want ErrNoJSON, got %v", err)
	}
}

// A story reading in the wrong shape is an error; a good one keeps only watchlist tickers, once each.
func TestReadStoryValidates(t *testing.T) {
	answer := `{"events":"one","tickers":["nvda","NVDA","ZZZZ"],"direction":"down","importance":4,"summary":"Nvidia's board weighs ousting its chief."}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"content": answer}})
	}))
	defer srv.Close()
	o := Ollama{Host: srv.URL, HTTP: srv.Client()}
	in := StoryInput{Headlines: []string{"Nvidia board weighs ousting Huang"}, Watchlist: map[string]string{"NVDA": "nvidia", "AMD": "amd"}, Instrument: "NVDA"}
	r, _, err := o.ReadStory(t.Context(), TEST_MODEL, "", in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tickers) != 1 || r.Tickers[0] != "NVDA" || r.Direction != DIRECTION_DOWN {
		t.Fatalf("reading wrong: %+v", r)
	}
	answer = `{"events":"maybe","tickers":[],"direction":"up","importance":9,"summary":""}`
	if _, _, err := o.ReadStory(t.Context(), TEST_MODEL, "", in); err == nil {
		t.Fatal("a reading in the wrong shape must be an error")
	}
}
