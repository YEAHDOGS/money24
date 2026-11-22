// Package localai talks to models that run on this machine: Ollama for language models. It is a reader,
// never a trader: what a model says is information filed on a story or a play, with the model's name and
// its words, and nothing in the engine places an order because of it.
//
// It uses Ollama's native /api/chat rather than its OpenAI-compatible route, because only the native
// route takes the context window (Ollama's default of 4096 tokens cuts a play's report short), memory
// mapping (gpt-oss:20b does not load on a 6 GB GPU without it), the thinking level, and reports how long
// loading, reading and writing took.
package localai

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	ENV_OLLAMA_HOST      = "OLLAMA_HOST"       // Ollama's own variable; may be a bind address such as 0.0.0.0:11434
	ENV_OLLAMA_MODEL     = "M24D_OLLAMA_MODEL" // which model reads plays
	ENV_OLLAMA_THINK     = "M24D_OLLAMA_THINK" // low, medium, high, true or false; empty is the model's default
	DEFAULT_OLLAMA_HOST  = "http://127.0.0.1:11434"
	DEFAULT_OLLAMA_MODEL = "gpt-oss:20b"
	DEFAULT_OLLAMA_PORT  = "11434"
	DEFAULT_SCHEME       = "http"
	ANY_ADDRESS_V4       = "0.0.0.0" // a bind address; a client reaches it on loopback
	ANY_ADDRESS_V6       = "::"
	LOOPBACK             = "127.0.0.1"

	CHAT_PATH = "/api/chat"
	TAGS_PATH = "/api/tags"
	PS_PATH   = "/api/ps"

	PROBE_TIMEOUT = 1500 * time.Millisecond // Ollama on loopback answers /api/tags in milliseconds
	PROBE_TTL     = 20 * time.Second        // how long an up-or-down answer is reused
	MAX_BODY      = 8 << 20
	MAX_ERROR     = 300
	DEFAULT_CTX   = 16384 // tokens: a play with its report and headlines fits with room for the answer
	// Ollama copies the part of a model that does not fit on the GPU into pinned memory unless it is
	// memory-mapped, and on a 6 GB laptop GPU that copy fails for gpt-oss:20b. Mapping costs nothing
	// for a model that fits.
	USE_MMAP = true
	// The JSON shape goes into the prompt as well as the format field: some runners (gpt-oss on
	// llama.cpp) do not enforce the format, so the model has to be told.
	SCHEMA_PROMPT = "\n\nAnswer with one JSON object and nothing else. It must match this JSON schema:\n"
	THINK_TRUE    = "true"
	THINK_FALSE   = "false"
)

var (
	ErrDown       = errors.New("Ollama is not answering")
	ErrNoModel    = errors.New("that model is not pulled in Ollama")
	ErrNoJSON     = errors.New("the answer holds no JSON object")
	THINK_LEVELS  = map[string]bool{"low": true, "medium": true, "high": true}
	probeMu       sync.Mutex
	probeCache    = map[string]probe{}
	defaultClient = &http.Client{}
)

type probe struct {
	at  time.Time
	err error
}

// Enabled reports whether the owner chose local AI in .env: OLLAMA_HOST or M24D_OLLAMA_MODEL is set.
// Without either, the engine never calls Ollama, so a server without it behaves as it always has.
func Enabled() bool {
	return os.Getenv(ENV_OLLAMA_HOST) != "" || os.Getenv(ENV_OLLAMA_MODEL) != ""
}

// Model is the model that reads plays: M24D_OLLAMA_MODEL, else gpt-oss:20b.
func Model() string {
	return cmp.Or(strings.TrimSpace(os.Getenv(ENV_OLLAMA_MODEL)), DEFAULT_OLLAMA_MODEL)
}

// Host turns OLLAMA_HOST into a URL a client can call. Ollama reads the same variable as the address it
// listens on, so "0.0.0.0", ":11434" and "host" without a scheme or port all have to work.
func Host(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DEFAULT_OLLAMA_HOST
	}
	if !strings.Contains(raw, "://") {
		raw = DEFAULT_SCHEME + "://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return DEFAULT_OLLAMA_HOST
	}
	host, port := u.Hostname(), u.Port()
	if host == "" || host == ANY_ADDRESS_V4 || host == ANY_ADDRESS_V6 {
		host = LOOPBACK
	}
	u.Host = net.JoinHostPort(host, cmp.Or(port, DEFAULT_OLLAMA_PORT))
	return strings.TrimRight(u.String(), "/")
}

// Ollama is one Ollama server.
type Ollama struct {
	Host string
	HTTP *http.Client
}

// FromEnv is the Ollama named in .env, or the default address.
func FromEnv(client *http.Client) Ollama {
	return Ollama{Host: Host(os.Getenv(ENV_OLLAMA_HOST)), HTTP: cmp.Or(client, defaultClient)}
}

// Up asks Ollama for its model list with a short timeout. It returns ErrDown, wrapped with the address,
// when nothing answers.
func (o Ollama) Up(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, PROBE_TIMEOUT)
	defer cancel()
	_, err := o.Models(ctx)
	return err
}

// UpCached is Up, reusing the last answer for PROBE_TTL so a page that polls providers does not probe each time.
func (o Ollama) UpCached(ctx context.Context) error {
	probeMu.Lock()
	last, ok := probeCache[o.Host]
	probeMu.Unlock()
	if ok && time.Since(last.at) < PROBE_TTL {
		return last.err
	}
	err := o.Up(ctx)
	probeMu.Lock()
	probeCache[o.Host] = probe{at: time.Now(), err: err}
	probeMu.Unlock()
	return err
}

// Models lists the models Ollama has pulled.
func (o Ollama) Models(ctx context.Context) ([]string, error) {
	var res struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := o.call(ctx, http.MethodGet, TAGS_PATH, nil, &res); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(res.Models))
	for _, m := range res.Models {
		out = append(out, m.Name)
	}
	return out, nil
}

// Loaded is one model Ollama holds in memory, and how much of it sits on the GPU.
type Loaded struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	SizeVRAM int64  `json:"size_vram"`
	Context  int    `json:"context_length"`
}

// Running lists the models Ollama has loaded right now.
func (o Ollama) Running(ctx context.Context) ([]Loaded, error) {
	var res struct {
		Models []Loaded `json:"models"`
	}
	if err := o.call(ctx, http.MethodGet, PS_PATH, nil, &res); err != nil {
		return nil, err
	}
	return res.Models, nil
}

// Unload asks Ollama to free a model's memory now instead of after its keep-alive.
func (o Ollama) Unload(ctx context.Context, model string) error {
	body := map[string]any{"model": model, "messages": []any{}, "keep_alive": 0}
	return o.call(ctx, http.MethodPost, CHAT_PATH, body, &struct{}{})
}

// Request is one question to one model.
type Request struct {
	Model     string
	System    string
	User      string
	Schema    map[string]any // the JSON shape of the answer; nil for free text
	Think     string         // low, medium, high, true, false; empty is the model's default
	NumCtx    int            // context window in tokens; 0 is DEFAULT_CTX
	MaxTokens int            // cap on the answer, thinking included; 0 is no cap
}

// Result is the model's answer and what it cost.
type Result struct {
	Model         string  `json:"model"`
	Content       string  `json:"content"`
	Thinking      string  `json:"thinking,omitempty"`
	PromptTokens  int     `json:"prompt_tokens"`
	OutputTokens  int     `json:"output_tokens"` // thinking included
	LoadSeconds   float64 `json:"load_seconds"`
	PromptSeconds float64 `json:"prompt_seconds"`
	EvalSeconds   float64 `json:"eval_seconds"`
	TotalSeconds  float64 `json:"total_seconds"`
	DoneReason    string  `json:"done_reason"`
}

// TokensPerSecond is the writing speed: output tokens over the time spent writing them.
func (r Result) TokensPerSecond() float64 {
	if r.EvalSeconds <= 0 {
		return 0
	}
	return float64(r.OutputTokens) / r.EvalSeconds
}

// thinkValue turns the Think setting into what Ollama takes: a level string, a boolean, or nothing.
func thinkValue(think string) any {
	think = strings.ToLower(strings.TrimSpace(think))
	switch {
	case THINK_LEVELS[think]:
		return think
	case think == THINK_TRUE:
		return true
	case think == THINK_FALSE:
		return false
	}
	return nil
}

// Chat asks one model one question and waits for the whole answer. The caller's context sets the deadline.
func (o Ollama) Chat(ctx context.Context, req Request) (Result, error) {
	system := req.System
	options := map[string]any{"num_ctx": cmp.Or(req.NumCtx, DEFAULT_CTX), "use_mmap": USE_MMAP, "temperature": 0}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}
	body := map[string]any{"model": req.Model, "stream": false, "options": options}
	if req.Schema != nil {
		shape, err := json.Marshal(req.Schema)
		if err != nil {
			return Result{}, err
		}
		body["format"] = req.Schema
		system += SCHEMA_PROMPT + string(shape)
	}
	body["messages"] = []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": req.User}}
	if think := thinkValue(req.Think); think != nil {
		body["think"] = think
	}
	var res struct {
		Model   string `json:"model"`
		Message struct {
			Content  string `json:"content"`
			Thinking string `json:"thinking"`
		} `json:"message"`
		DoneReason      string `json:"done_reason"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
		LoadDuration    int64  `json:"load_duration"`
		PromptDuration  int64  `json:"prompt_eval_duration"`
		EvalDuration    int64  `json:"eval_duration"`
		TotalDuration   int64  `json:"total_duration"`
	}
	if err := o.call(ctx, http.MethodPost, CHAT_PATH, body, &res); err != nil {
		return Result{}, err
	}
	return Result{
		Model: cmp.Or(res.Model, req.Model), Content: res.Message.Content, Thinking: res.Message.Thinking,
		PromptTokens: res.PromptEvalCount, OutputTokens: res.EvalCount, DoneReason: res.DoneReason,
		LoadSeconds: time.Duration(res.LoadDuration).Seconds(), PromptSeconds: time.Duration(res.PromptDuration).Seconds(),
		EvalSeconds: time.Duration(res.EvalDuration).Seconds(), TotalSeconds: time.Duration(res.TotalDuration).Seconds(),
	}, nil
}

// call sends one request. A refused or timed-out connection becomes ErrDown; a model Ollama does not
// have becomes ErrNoModel; any other failure carries Ollama's own words.
func (o Ollama) call(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, o.Host+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := cmp.Or(o.HTTP, defaultClient).Do(req)
	if err != nil {
		if ctx.Err() != nil && path == CHAT_PATH {
			return fmt.Errorf("Ollama at %s took too long: %w", o.Host, ctx.Err())
		}
		return fmt.Errorf("%w at %s: %v", ErrDown, o.Host, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, MAX_BODY))
	if err != nil {
		return err
	}
	if res.StatusCode == http.StatusNotFound && path == CHAT_PATH {
		return fmt.Errorf("%w: %s", ErrNoModel, shorten(data))
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama HTTP %d: %s", res.StatusCode, shorten(data))
	}
	return json.Unmarshal(data, out)
}

func shorten(data []byte) string {
	msg := strings.TrimSpace(string(data))
	if len(msg) > MAX_ERROR {
		return msg[:MAX_ERROR]
	}
	return msg
}

// DecodeJSON reads the first JSON object in a model's answer into out. Models that ignore the format
// field wrap the object in a code fence or a sentence; the object itself is what counts.
func DecodeJSON(content string, out any) error {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return ErrNoJSON
	}
	return json.Unmarshal([]byte(content[start:end+1]), out)
}
