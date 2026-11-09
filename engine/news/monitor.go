package news

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	EWMA_WEIGHT    = 0.2 // how fast the per-request average follows new requests
	CHANNEL_WIRE   = "_wire"
	CHANNEL_SERVED = "_served"
	DAY_LAYOUT     = "2006-01-02"
	MAX_ERROR_TEXT = 200
)

// Usage is bytes in and out for one request or one total.
type Usage struct {
	In  int64 `json:"in"`
	Out int64 `json:"out"`
}

// Tally is a request count plus bytes.
type Tally struct {
	Requests int64 `json:"requests"`
	In       int64 `json:"in"`
	Out      int64 `json:"out"`
}

func (t *Tally) add(other Tally) {
	t.Requests += other.Requests
	t.In += other.In
	t.Out += other.Out
}

// SourceStats is the live meter for one source. Bytes are compressed body plus headers.
type SourceStats struct {
	ID          string     `json:"id"`
	Label       string     `json:"label"`
	Kind        string     `json:"kind"`
	MinEveryMs  int64      `json:"min_every_ms"`
	Requests    int64      `json:"requests"`
	NotModified int64      `json:"not_modified"`
	Errors      int64      `json:"errors"`
	BytesIn     int64      `json:"bytes_in"`
	BytesOut    int64      `json:"bytes_out"`
	LastIn      int64      `json:"last_in"`
	LastOut     int64      `json:"last_out"`
	AvgIn       float64    `json:"avg_in"`
	AvgOut      float64    `json:"avg_out"`
	LastStatus  int        `json:"last_status"`
	LastError   string     `json:"last_error,omitempty"`
	LastAt      *time.Time `json:"last_at"`
	LastMs      int64      `json:"last_ms"`
	LastAdded   int        `json:"last_added"`
	Added       int64      `json:"added"`
	version     uint64
}

// Monitor meters every request, the raw sockets under them, and what we serve to clients.
type Monitor struct {
	mu        sync.Mutex
	started   time.Time
	order     []string
	sources   map[string]*SourceStats
	version   uint64
	pending   map[string]*Tally
	wireIn    atomic.Int64
	wireOut   atomic.Int64
	served    atomic.Int64
	servedReq atomic.Int64
	flushed   struct{ wireIn, wireOut, served, servedReq int64 }
}

// NewMonitor registers every source so the UI can list them before their first poll.
func NewMonitor(sources []Source) *Monitor {
	m := &Monitor{started: time.Now().UTC(), sources: map[string]*SourceStats{}, pending: map[string]*Tally{}}
	for _, src := range sources {
		m.order = append(m.order, src.ID)
		m.sources[src.ID] = &SourceStats{ID: src.ID, Label: src.Label, Kind: src.Kind, MinEveryMs: src.MinEvery.Milliseconds()}
	}
	return m
}

func ewma(avg float64, sample int64, first bool) float64 {
	if first {
		return float64(sample)
	}
	return avg + EWMA_WEIGHT*(float64(sample)-avg)
}

func (m *Monitor) recordRequest(sourceID string, u Usage, status int, took time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sources[sourceID]
	if !ok {
		return
	}
	now := time.Now().UTC()
	first := s.Requests == 0
	s.Requests++
	s.BytesIn += u.In
	s.BytesOut += u.Out
	s.LastIn, s.LastOut = u.In, u.Out
	s.AvgIn = ewma(s.AvgIn, u.In, first)
	s.AvgOut = ewma(s.AvgOut, u.Out, first)
	s.LastStatus = status
	s.LastAt = &now
	s.LastMs = took.Milliseconds()
	s.LastError = ""
	if status == 304 {
		s.NotModified++
	}
	if err != nil {
		s.Errors++
		s.LastError = truncate(err.Error(), MAX_ERROR_TEXT)
	}
	m.touch(s)

	t := m.pendingFor(sourceID)
	t.add(Tally{Requests: 1, In: u.In, Out: u.Out})
}

func (m *Monitor) recordAdded(sourceID string, added int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sources[sourceID]
	if !ok {
		return
	}
	s.LastAdded = added
	s.Added += int64(added)
	m.touch(s)
}

// recordServed counts bytes we send to UI and API clients: the real egress on a hosted server.
func (m *Monitor) recordServed(bytes int64, newRequest bool) {
	m.served.Add(bytes)
	if newRequest {
		m.servedReq.Add(1)
	}
}

func (m *Monitor) touch(s *SourceStats) {
	m.version++
	s.version = m.version
}

func (m *Monitor) pendingFor(channel string) *Tally {
	t, ok := m.pending[channel]
	if !ok {
		t = &Tally{}
		m.pending[channel] = t
	}
	return t
}

// drain hands over every byte counted since the last call, keyed by channel, for the daily ledger.
func (m *Monitor) drain() map[string]Tally {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]Tally{}
	for channel, t := range m.pending {
		out[channel] = *t
	}
	m.pending = map[string]*Tally{}

	wireIn, wireOut := m.wireIn.Load(), m.wireOut.Load()
	served, servedReq := m.served.Load(), m.servedReq.Load()
	out[CHANNEL_WIRE] = Tally{In: wireIn - m.flushed.wireIn, Out: wireOut - m.flushed.wireOut}
	out[CHANNEL_SERVED] = Tally{Requests: servedReq - m.flushed.servedReq, Out: served - m.flushed.served}
	m.flushed.wireIn, m.flushed.wireOut, m.flushed.served, m.flushed.servedReq = wireIn, wireOut, served, servedReq
	return out
}

// NetSnapshot is what the network monitor shows. Sources holds only those changed after Since.
type NetSnapshot struct {
	Version    uint64           `json:"version"`
	Since      uint64           `json:"since"`
	StartedAt  time.Time        `json:"started_at"`
	Now        time.Time        `json:"now"`
	EveryMs    int64            `json:"every_ms"`
	Clients    int              `json:"clients"`
	Wire       Usage            `json:"wire"`
	Served     Tally            `json:"served"`
	Fetched    Tally            `json:"fetched"`
	Month      map[string]Tally `json:"month"`
	MonthStart string           `json:"month_start"`
	Sources    []SourceStats    `json:"sources"`
}

func (m *Monitor) snapshot(since uint64) NetSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := NetSnapshot{
		Version:   m.version,
		Since:     since,
		StartedAt: m.started,
		Now:       time.Now().UTC(),
		Wire:      Usage{In: m.wireIn.Load(), Out: m.wireOut.Load()},
		Served:    Tally{Requests: m.servedReq.Load(), Out: m.served.Load()},
		Sources:   []SourceStats{},
	}
	for _, id := range m.order {
		s := m.sources[id]
		snap.Fetched.add(Tally{Requests: s.Requests, In: s.BytesIn, Out: s.BytesOut})
		if s.version > since || since == 0 {
			snap.Sources = append(snap.Sources, *s)
		}
	}
	return snap
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
