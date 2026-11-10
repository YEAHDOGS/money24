package news

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	SSE_HEARTBEAT        = 25 * time.Second
	SSE_RETRY_MS         = 3000
	DEFAULT_LIMIT        = 100
	BACKFILL_LIMIT       = 300
	MAX_SETTINGS_BODY    = 1 << 10
	PREFLIGHT_MAX_AGE    = "600"
	ALLOWED_METHODS      = "GET, PUT, OPTIONS"
	ALLOWED_HEADERS      = "Content-Type, Authorization, Last-Event-ID"
	BEARER_PREFIX        = "Bearer "
	STATUS_LINE_ESTIMATE = len("HTTP/1.1 200 OK\r\n\r\n")
)

// HTTPOptions guards the API. Origins are the only browser origins allowed to call it.
// Hosts, when set, is the only Host header accepted (DNS-rebinding defence on localhost).
// AdminToken, when set, is required to change settings (use it on a server).
type HTTPOptions struct {
	Origins    []string
	Hosts      []string
	AdminToken string
}

type api struct {
	svc     *Service
	origins map[string]bool
	hosts   map[string]bool
	token   string
}

func toSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			set[v] = true
		}
	}
	return set
}

// Handler serves the JSON API and the live stream.
func (s *Service) Handler(opts HTTPOptions) http.Handler {
	a := &api{svc: s, origins: toSet(opts.Origins), hosts: toSet(opts.Hosts), token: opts.AdminToken}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/events", a.events)
	mux.HandleFunc("GET /api/stream", a.stream)
	mux.HandleFunc("GET /api/network", a.network)
	mux.HandleFunc("GET /api/settings", a.getSettings)
	mux.HandleFunc("PUT /api/settings", a.putSettings)
	return a.meter(a.guard(mux))
}

// guard enforces Host, CORS and write-origin rules before any route runs.
func (a *api) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(a.hosts) > 0 && !a.hosts[r.Host] {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		origin := r.Header.Get("Origin")
		allowed := a.origins[origin]
		if allowed {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", ALLOWED_METHODS)
			h.Set("Access-Control-Allow-Headers", ALLOWED_HEADERS)
			h.Set("Access-Control-Max-Age", PREFLIGHT_MAX_AGE)
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet && origin != "" && !allowed {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// meter counts every byte we send back: the egress a hosted server would pay for.
func (a *api) meter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.svc.monitor.recordServed(0, true)
		next.ServeHTTP(&meteredWriter{ResponseWriter: w, monitor: a.svc.monitor}, r)
	})
}

type meteredWriter struct {
	http.ResponseWriter
	monitor     *Monitor
	wroteHeader bool
}

func (w *meteredWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		size := STATUS_LINE_ESTIMATE
		for key, values := range w.Header() {
			for _, v := range values {
				size += len(key) + len(v) + HEADER_LINE_EXTRA
			}
		}
		w.monitor.recordServed(int64(size), false)
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *meteredWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.monitor.recordServed(int64(n), false)
	return n, err
}

func (w *meteredWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *meteredWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func queryInt(r *http.Request, key string, fallback int64) int64 {
	n, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func (a *api) events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	events, err := a.svc.store.events(r.Context(), EventQuery{
		Limit:    int(queryInt(r, "limit", DEFAULT_LIMIT)),
		BeforeID: queryInt(r, "before", 0),
		Kind:     q.Get("kind"),
		Tag:      q.Get("tag"),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (a *api) network(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.svc.Snapshot(r.Context(), uint64(queryInt(r, "since", 0))))
}

func (a *api) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.svc.Settings())
}

func (a *api) putSettings(w http.ResponseWriter, r *http.Request) {
	if a.token != "" {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), BEARER_PREFIX)
		if subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin token required"})
			return
		}
	}
	var body struct {
		PollEveryMs int64 `json:"poll_every_ms"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MAX_SETTINGS_BODY)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be {\"poll_every_ms\": n}"})
		return
	}
	settings, err := a.svc.SetPollEvery(r.Context(), time.Duration(body.PollEveryMs)*time.Millisecond)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func writeSSE(w http.ResponseWriter, msg Message) {
	if msg.Event != "" {
		fmt.Fprintf(w, "event: %s\n", msg.Event)
	}
	if msg.ID > 0 {
		fmt.Fprintf(w, "id: %d\n", msg.ID)
	}
	fmt.Fprintf(w, "data: %s\n\n", msg.Data)
}

// stream is the live feed: settings and a full meter reading first, then any events the client
// missed (from Last-Event-ID or ?after=), then every new event and meter change as it happens.
func (a *api) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	c := a.svc.hub.add()
	defer a.svc.hub.remove(c)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // keep reverse proxies from buffering the stream
	fmt.Fprintf(w, "retry: %d\n\n", SSE_RETRY_MS)

	settings, _ := json.Marshal(a.svc.Settings())
	writeSSE(w, Message{Event: EVENT_SETTINGS, Data: settings})
	stats, _ := json.Marshal(a.svc.Snapshot(ctx, 0))
	writeSSE(w, Message{Event: EVENT_STATS, Data: stats})
	plan, report := a.svc.clock.snapshot()
	planData, _ := json.Marshal(plan)
	writeSSE(w, Message{Event: EVENT_PLAN, Data: planData})
	if report != nil {
		reportData, _ := json.Marshal(report)
		writeSSE(w, Message{Event: EVENT_CYCLE, Data: reportData})
	}

	lastSent, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if err != nil {
		lastSent = queryInt(r, "after", 0)
	}
	if lastSent > 0 {
		missed, _ := a.svc.store.events(ctx, EventQuery{AfterID: lastSent, Limit: BACKFILL_LIMIT})
		for i := len(missed) - 1; i >= 0; i-- {
			data, _ := json.Marshal(missed[i])
			writeSSE(w, Message{ID: missed[i].ID, Data: data})
			lastSent = max(lastSent, missed[i].ID)
		}
	}
	flusher.Flush()

	heartbeat := time.NewTicker(SSE_HEARTBEAT)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, open := <-c:
			if !open {
				return // fell behind; the browser reconnects and backfills
			}
			if msg.ID > 0 && msg.ID <= lastSent {
				continue
			}
			writeSSE(w, msg)
			lastSent = max(lastSent, msg.ID)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
