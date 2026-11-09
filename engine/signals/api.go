package signals

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DEFAULT_JOURNAL_LIMIT = 200
	SSE_HEARTBEAT         = 25 * time.Second
	SSE_RETRY_MS          = 3000
	API_METHODS           = "GET, POST, OPTIONS"
	API_HEADERS           = "Content-Type, Authorization, Last-Event-ID"
	API_PREFLIGHT_MAX_AGE = "600"
)

// APIOptions guards the read API the UI uses. Origins are the browser origins allowed to call it;
// Hosts, when set, are the only Host headers accepted (DNS-rebinding defence on localhost).
type APIOptions struct {
	Origins []string
	Hosts   []string
}

// TrustsLocalDesk lets the desk UI post signals without the secret, but only when the runner is bound
// to loopback (Hosts set) and the request comes from an allowed browser origin on a loopback Host.
// Anything that can make such a request can already read the secret file on the same machine.
func (o APIOptions) TrustsLocalDesk(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if len(o.Hosts) == 0 || origin == "" {
		return false
	}
	hostOK, originOK := false, false
	for _, h := range o.Hosts {
		hostOK = hostOK || strings.TrimSpace(h) == r.Host
	}
	for _, o := range o.Origins {
		originOK = originOK || strings.TrimSpace(o) == origin
	}
	return hostOK && originOK
}

// API serves the trading UI: GET /api/journal, GET /api/stream (SSE), GET /api/sources, plus the
// webhook at POST /signals and GET /healthz. Headless servers serve the same routes; the UI is optional.
func API(j *Journal, c *Catalog, webhook http.Handler, opts APIOptions) http.Handler {
	origins, hosts := map[string]bool{}, map[string]bool{}
	for _, o := range opts.Origins {
		origins[strings.TrimSpace(o)] = true
	}
	for _, h := range opts.Hosts {
		hosts[strings.TrimSpace(h)] = true
	}

	mux := http.NewServeMux()
	mux.Handle("POST /signals", webhook)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/sources", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, c) })
	mux.HandleFunc("GET /api/journal", func(w http.ResponseWriter, r *http.Request) {
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil || limit < 1 || limit > JOURNAL_RECENT {
			limit = DEFAULT_JOURNAL_LIMIT
		}
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		writeJSON(w, j.Recent(after, limit))
	})
	mux.HandleFunc("GET /api/stream", func(w http.ResponseWriter, r *http.Request) { stream(w, r, j) })

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(hosts) > 0 && !hosts[r.Host] {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if !origins[origin] {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", API_METHODS)
				h.Set("Access-Control-Allow-Headers", API_HEADERS)
				h.Set("Access-Control-Max-Age", API_PREFLIGHT_MAX_AGE)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// stream sends every new journal entry as one SSE message with its id, after backfilling anything
// newer than Last-Event-ID (or ?after=), so a UI that slept or lost the network misses nothing.
func stream(w http.ResponseWriter, r *http.Request, j *Journal) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if after == 0 {
		after, _ = strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	}
	live, done := j.Subscribe()
	defer done()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, "retry: %d\n\n", SSE_RETRY_MS)
	send := func(e Entry) {
		b, _ := json.Marshal(e)
		fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.ID, b)
		after = max(after, e.ID)
	}
	for _, e := range j.Recent(after, JOURNAL_RECENT) {
		send(e)
	}
	flusher.Flush()

	beat := time.NewTicker(SSE_HEARTBEAT)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-live:
			if e.ID > after {
				send(e)
				flusher.Flush()
			}
		case <-beat.C:
			fmt.Fprint(w, ": beat\n\n")
			flusher.Flush()
		}
	}
}
