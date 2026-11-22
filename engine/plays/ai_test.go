package plays

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A fake Claude endpoint: checks the request is a keyed, schema-constrained, tool-free call and
// answers with a structured analysis, which must come back parsed with its evidence intact.
func TestAnalyzeAnthropicRequestAndParse(t *testing.T) {
	answer := `{"verdict":"wait","summary":"s","must_happen":"AAL at or below 12.30 by Thu 3 PM","thesis":"t","risks":["iv"],` +
		`"evidence":[{"kind":"news","label":"FAA ground stop","stance":"for","source":"x","url":"","breaking":true,"source_count":3}],` +
		`"suggestion":{"entry":0.13,"stop":0.07,"ladder":[{"qty":4,"price":0.26}],"runner_qty":3,"runner_target":1.04,"exit_by":""},"confidence":0.4}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing auth or version headers")
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		if _, ok := req["tools"]; ok {
			t.Error("the analyst must have no tools")
		}
		format, _ := req["output_config"].(map[string]any)["format"].(map[string]any)
		if format["type"] != "json_schema" || req["model"] != "claude-opus-5-5" {
			t.Errorf("want a json_schema call on claude-opus-5-5, got %v / %v", format["type"], req["model"])
		}
		json.NewEncoder(w).Encode(map[string]any{"stop_reason": "end_turn", "content": []map[string]any{{"type": "text", "text": answer}}})
	}))
	defer srv.Close()
	saved := PROVIDERS[0].URL
	PROVIDERS[0].URL = srv.URL
	defer func() { PROVIDERS[0].URL = saved }()
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("M24D_ANTHROPIC_MODEL", "")

	p := aalPut()
	a, err := Analyze(t.Context(), srv.Client(), PROVIDER_ANTHROPIC, p, Build(p, DOGSRules(), Book{Now: TEST_NOW}), nil, "AAL 12.5P 11/20")
	if err != nil {
		t.Fatal(err)
	}
	if a.Verdict != "wait" || len(a.Evidence) != 1 || !a.Evidence[0].Breaking || a.Suggestion.RunnerTarget != 1.04 {
		t.Fatalf("parsed wrong: %+v", a)
	}
	if !strings.Contains(a.MustHappen, "12.30") || a.Provider != PROVIDER_ANTHROPIC {
		t.Errorf("must_happen or provider lost: %+v", a)
	}
}

// AI news never moves the odds until a person verifies it.
func TestUnverifiedAINewsIsIgnored(t *testing.T) {
	p := aalPut()
	base := Build(p, DOGSRules(), Book{Now: TEST_NOW}).Odds.Score
	p.Evidence = append(p.Evidence, Evidence{Kind: EV_NEWS, Label: "FAA", Stance: STANCE_FOR, Breaking: true, SourceCount: 5, TrustPct: 90, AI: true})
	o := Build(p, DOGSRules(), Book{Now: TEST_NOW}).Odds
	if o.Score != base || len(o.Ignored) != 1 {
		t.Fatalf("unverified AI news moved the score %.1f → %.1f", base, o.Score)
	}
	p.Evidence[len(p.Evidence)-1].Verified = true
	if Build(p, DOGSRules(), Book{Now: TEST_NOW}).Odds.Score <= base {
		t.Error("verified, trusted, breaking news for the play should raise the score")
	}
}

// A fake Ollama: the local provider is connected only once .env names it, asks with the analysis schema
// and no key, and asks a second time when the first answer comes back in the wrong shape.
func TestAnalyzeOllamaRetriesWrongShape(t *testing.T) {
	answer := `{"verdict":"pass","summary":"thin","must_happen":"AAL under 12.30 by Thu","thesis":"t","risks":["spread"],"evidence":[],` +
		`"suggestion":{"entry":0,"stop":0,"ladder":[],"runner_qty":0,"runner_target":0,"exit_by":""},"confidence":0.2}`
	asks := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "gpt-oss:20b"}}})
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("the local provider sends no key")
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		if _, ok := req["format"].(map[string]any); !ok || req["model"] != "gpt-oss:20b" {
			t.Errorf("want a schema call on gpt-oss:20b, got %v", req["model"])
		}
		asks++
		content := answer
		if asks == 1 {
			content = "I think you should wait."
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "gpt-oss:20b", "message": map[string]any{"content": content}})
	}))
	defer srv.Close()
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("M24D_OLLAMA_MODEL", "")
	if p := ollamaProvider(t); p.Connected || p.Detail != OLLAMA_OFF {
		t.Fatalf("Ollama must stay off until .env names it: %+v", p)
	}
	t.Setenv("OLLAMA_HOST", srv.URL)
	if p := ollamaProvider(t); !p.Connected || !p.Local || p.Model != "gpt-oss:20b" {
		t.Fatalf("Ollama should be connected: %+v", p)
	}
	p := aalPut()
	a, err := Analyze(t.Context(), srv.Client(), PROVIDER_OLLAMA, p, Build(p, DOGSRules(), Book{Now: TEST_NOW}), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if asks != 2 || a.Verdict != "pass" || a.Provider != PROVIDER_OLLAMA || a.Model != "gpt-oss:20b" {
		t.Fatalf("asks=%d analysis=%+v", asks, a)
	}
}

// Chosen in .env but not running: not connected, with the reason, and an explicit ask says why.
func TestAnalyzeOllamaDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	t.Setenv("OLLAMA_HOST", addr)
	prov := ollamaProvider(t)
	if prov.Connected || !strings.Contains(prov.Detail, "not answering") {
		t.Fatalf("a stopped Ollama must show as not connected with the reason: %+v", prov)
	}
	p := aalPut()
	_, err = Analyze(t.Context(), http.DefaultClient, PROVIDER_OLLAMA, p, Build(p, DOGSRules(), Book{Now: TEST_NOW}), nil, "")
	if err == nil || !strings.Contains(err.Error(), "tray icon") {
		t.Fatalf("want the down reason, got %v", err)
	}
}

func ollamaProvider(t *testing.T) Provider {
	t.Helper()
	for _, p := range Providers() {
		if p.ID == PROVIDER_OLLAMA {
			return p
		}
	}
	t.Fatal("no Ollama provider")
	return Provider{}
}
