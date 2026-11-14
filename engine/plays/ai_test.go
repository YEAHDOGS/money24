package plays

import (
	"encoding/json"
	"io"
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
