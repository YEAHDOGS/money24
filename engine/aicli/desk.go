package aicli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	ROUTE_STATUS = "GET /api/ai"
	ROUTE_TEST   = "POST /api/ai/test"
	ROUTE_IMPACT = "POST /api/ai/impact"
	WORK_SUBDIR  = "ai-work"
	// A tiny question at start and every half hour: shows who is signed in and how fast each answers,
	// for a few dozen small asks a day against the user's plans.
	PROBE_EVERY  = 30 * time.Minute
	PROBE_PROMPT = `Reply with {"ok": true}.`
	MAX_BODY     = 64 << 10
	KEEP_IMPACTS = 20
	HEADER_TYPE  = "Content-Type"
	HEADER_CACHE = "Cache-Control"
	JSON_TYPE    = "application/json"
	NO_STORE     = "no-store"
)

var ErrNotAllowed = errors.New("only the desk on this machine can ask the AIs")

// AppStatus is what the desk last saw from one app. No paths or account details leave the runner.
type AppStatus struct {
	App
	Installed bool      `json:"installed"`
	State     string    `json:"state"` // ok, sign in, out of usage, not installed, failed, too slow, or empty before the first ask
	Millis    int64     `json:"ms"`    // the last answer's time, start to finish
	At        time.Time `json:"at,omitzero"`
	Error     string    `json:"error,omitempty"`
}

// ImpactRun is one news story put to every app: who agreed, on what, and how fast.
type ImpactRun struct {
	At        time.Time `json:"at"`
	News      string    `json:"news"`
	Agreed    bool      `json:"agreed"`
	Consensus *Impact   `json:"consensus,omitempty"`
	DecidedMs int64     `json:"decided_ms"` // until the quorum agreed, or until every app had answered
	Answers   []Answer  `json:"answers"`
}

// Desk keeps each app's status, probes them on a timer and answers the runner's AI routes.
type Desk struct {
	Asker    *Asker
	MayWrite func(*http.Request) bool // the local desk only: asks spend the user's plan

	mu      sync.Mutex
	status  map[string]*AppStatus
	impacts []*ImpactRun
}

// NewDesk uses every installed app (or those named in M24D_AI_APPS), working in dataDir/ai-work.
func NewDesk(dataDir string) *Desk {
	d := &Desk{Asker: &Asker{Apps: Selected(Apps()), WorkRoot: filepath.Join(dataDir, WORK_SUBDIR)}, status: map[string]*AppStatus{}}
	for _, app := range d.Asker.Apps {
		_, err := app.Path()
		d.status[app.ID] = &AppStatus{App: app, Installed: err == nil}
	}
	return d
}

// Run probes every app now and then every PROBE_EVERY until ctx ends.
func (d *Desk) Run(ctx context.Context) {
	for {
		d.Probe(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(PROBE_EVERY):
		}
	}
}

// Probe asks every app a tiny question and records who answered and how fast.
func (d *Desk) Probe(ctx context.Context) []Answer {
	q := Question{Prompt: PROBE_PROMPT, Schema: map[string]any{"type": "object", "additionalProperties": false,
		"required": []string{"ok"}, "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}}}
	return d.Asker.AskAll(ctx, q, func(a Answer) { d.record(a, true) })
}

// Impact puts one story to every app and decides as soon as IMPACT_NEED of them agree.
func (d *Desk) Impact(ctx context.Context, news string) ImpactRun {
	run := &ImpactRun{At: time.Now().UTC(), News: news}
	// The slower apps keep answering after the quorum (and after the request) for their record.
	ctx = context.WithoutCancel(ctx)
	group, all := d.Asker.Quorum(ctx, ImpactQuestion(news), IMPACT_NEED, ImpactsAgree, func(late Answer) {
		d.record(late, true)
		d.mu.Lock()
		run.Answers = append(run.Answers, late)
		d.mu.Unlock()
	})
	for _, ans := range all {
		d.record(ans, true)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	run.DecidedMs = time.Since(run.At).Milliseconds()
	run.Answers = append(run.Answers, all...)
	if shared, ok := Consensus(group); ok {
		run.Agreed, run.Consensus = true, &shared
	}
	d.impacts = append(d.impacts, run)
	if len(d.impacts) > KEEP_IMPACTS {
		d.impacts = d.impacts[len(d.impacts)-KEEP_IMPACTS:]
	}
	return *run
}

// Ask puts one question to one app, by id, and records whether it could answer. A deep ask's time is
// left out of the app's speed, which tracks quick asks only.
func (d *Desk) Ask(ctx context.Context, appID string, q Question) Answer {
	i := slices.IndexFunc(d.Asker.Apps, func(a App) bool { return a.ID == appID })
	if i < 0 {
		return Answer{App: appID, State: STATE_MISSING, Started: time.Now(), Error: "no AI app called " + appID}
	}
	one := Asker{Apps: d.Asker.Apps[i : i+1], WorkRoot: d.Asker.WorkRoot, Run: d.Asker.Run}
	ans := one.AskAll(ctx, q, nil)[0]
	d.record(ans, !q.Deep)
	return ans
}

// record files an answer on its app's status; speed says whether its time counts as the app's speed.
func (d *Desk) record(ans Answer, speed bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.status[ans.App]
	if st == nil {
		return
	}
	st.State, st.At, st.Error = ans.State, ans.Started.UTC(), ans.Error
	if speed {
		st.Millis = ans.Millis
	}
	st.Installed = ans.State != STATE_MISSING
	if ans.State != STATE_OK {
		log.Printf("AI %s: %s (%s)", st.Name, ans.State, firstLine(ans.Error))
	}
}

// Snapshot is the status route's body.
type Snapshot struct {
	Apps    []AppStatus `json:"apps"`
	Need    int         `json:"need"`
	Impacts []ImpactRun `json:"impacts"`
}

// Snapshot is every app as last seen, in order, and the recent impact runs.
func (d *Desk) Snapshot() Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	snap := Snapshot{Need: IMPACT_NEED, Apps: make([]AppStatus, 0, len(d.status)), Impacts: make([]ImpactRun, 0, len(d.impacts))}
	for _, app := range d.Asker.Apps {
		snap.Apps = append(snap.Apps, *d.status[app.ID])
	}
	for _, run := range slices.Backward(d.impacts) {
		snap.Impacts = append(snap.Impacts, *run)
	}
	return snap
}

// Routes: the status for anyone the runner already lets read, asks for the local desk only.
func (d *Desk) Routes() map[string]http.Handler {
	return map[string]http.Handler{
		ROUTE_STATUS: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, d.Snapshot()) }),
		ROUTE_TEST: d.guard(func(w http.ResponseWriter, r *http.Request) {
			d.Probe(r.Context())
			writeJSON(w, d.Snapshot())
		}),
		ROUTE_IMPACT: d.guard(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				News string `json:"news"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, MAX_BODY)).Decode(&body); err != nil || strings.TrimSpace(body.News) == "" {
				http.Error(w, "send {\"news\": \"the story's text\"}", http.StatusBadRequest)
				return
			}
			writeJSON(w, d.Impact(r.Context(), body.News))
		}),
	}
}

func (d *Desk) guard(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.MayWrite == nil || !d.MayWrite(r) {
			http.Error(w, ErrNotAllowed.Error(), http.StatusForbidden)
			return
		}
		h(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set(HEADER_TYPE, JSON_TYPE)
	w.Header().Set(HEADER_CACHE, NO_STORE)
	json.NewEncoder(w).Encode(v)
}
