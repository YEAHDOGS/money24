package signals

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// FOLLOW_ROUTE says how each kind of followed source is doing: the desktop app's pocket window
// reads it for the Telegram follower's last activity and today's count. Read-only.
const (
	FOLLOW_ROUTE  = "GET /api/follow"
	UTC_DAY       = "2006-01-02"
	KIND_SEPARATE = ":"
)

// FOLLOW_KINDS are the source kinds a reader polls on its own, in the order the route lists them.
var FOLLOW_KINDS = []string{TYPE_TELEGRAM, TYPE_DISCORD, TYPE_REDDIT}

// FollowFailure is one source whose last read failed.
type FollowFailure struct {
	Source string    `json:"source"`
	Error  string    `json:"error"`
	At     time.Time `json:"at"`
}

// FollowStatus is one kind of followed source: its newest post, how many were posted today (UTC,
// by the post's own time), and the sources whose last read failed.
type FollowStatus struct {
	Kind     string          `json:"kind"`
	Sources  int             `json:"sources"`             // in the catalog
	SignedIn bool            `json:"signed_in,omitempty"` // Telegram: read through a signed-in account
	LastAt   time.Time       `json:"last_at,omitzero"`
	Today    int             `json:"today"`
	Total    int             `json:"total"`
	Failing  []FollowFailure `json:"failing,omitempty"`
}

// followFailures remembers each source's last failed read until a read of it succeeds again.
type followFailures struct {
	mu   sync.Mutex
	byID map[string]FollowFailure
}

func (f *followFailures) note(source string, err error, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.byID, source)
		return
	}
	if f.byID == nil {
		f.byID = map[string]FollowFailure{}
	}
	f.byID[source] = FollowFailure{Source: source, Error: err.Error(), At: now.UTC()}
}

func (f *followFailures) of(kind string) []FollowFailure {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []FollowFailure{}
	for id, failure := range f.byID {
		if strings.HasPrefix(id, kind+KIND_SEPARATE) {
			out = append(out, failure)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Source < out[b].Source })
	return out
}

// Activity is one kind's recorded posts: the newest post's time, how many were posted on now's UTC
// day, and how many are on record.
func (r *Recorder) Activity(kind string, now time.Time) (last time.Time, today, total int) {
	day := now.UTC().Format(UTC_DAY)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.posts {
		if !strings.HasPrefix(strings.ToLower(p.SourceID), kind+KIND_SEPARATE) {
			continue
		}
		at := p.PostedAt
		if at.IsZero() {
			at = p.SeenAt
		}
		total++
		if at.UTC().Format(UTC_DAY) == day {
			today++
		}
		if at.After(last) {
			last = at
		}
	}
	return last, today, total
}

// FollowStatus lists every kind of followed source.
func (r *SignalReaders) FollowStatus(catalog *Catalog, rec *Recorder, now time.Time) []FollowStatus {
	out := make([]FollowStatus, 0, len(FOLLOW_KINDS))
	for _, kind := range FOLLOW_KINDS {
		s := FollowStatus{Kind: kind, SignedIn: kind == TYPE_TELEGRAM && r.telegram != nil, Failing: r.failures.of(kind)}
		for _, source := range catalog.Sources {
			if source.Type == kind {
				s.Sources++
			}
		}
		s.LastAt, s.Today, s.Total = rec.Activity(kind, now)
		out = append(out, s)
	}
	return out
}

// FollowRoute serves FollowStatus at FOLLOW_ROUTE.
func (r *SignalReaders) FollowRoute(catalog *Catalog, rec *Recorder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, r.FollowStatus(catalog, rec, time.Now()))
	})
}
