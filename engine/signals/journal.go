package signals

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

const (
	JOURNAL_RECENT   = 1000    // entries kept in memory for the UI and stream backfill
	JOURNAL_LINE_MAX = 1 << 20 // longest journal line read back on start
	JOURNAL_PERMS    = 0o644
	SUBSCRIBER_QUEUE = 64
)

// Entry is one journal line: a plan, a rejection, or an exit/trim/add the runner acts on.
type Entry struct {
	ID       int64     `json:"id"`
	At       time.Time `json:"at"`
	Action   string    `json:"action"`
	Signal   Signal    `json:"signal"`
	Plan     *Plan     `json:"plan,omitempty"`
	Result   *Result   `json:"result,omitempty"` // set on "close" lines
	Rejected string    `json:"rejected,omitempty"`
	Note     string    `json:"note,omitempty"` // what the gate changed or could not check
	// AsOf is the moment the plan was built for. Manual shares are planned as of the post time,
	// so an old callout shows what the rules would have done when it was posted.
	AsOf time.Time `json:"as_of,omitzero"`
}

// Journal is the append-only paper record. Every line also goes to live subscribers (the UI stream).
type Journal struct {
	mu     sync.Mutex
	file   *os.File
	enc    *json.Encoder
	recent []Entry
	nextID int64
	subs   map[chan Entry]struct{}
}

// OpenJournal opens path for appending and reads back its tail, so a restart keeps history and ids.
func OpenJournal(path string) (*Journal, error) {
	j := &Journal{nextID: 1, subs: map[chan Entry]struct{}{}}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, JOURNAL_LINE_MAX), JOURNAL_LINE_MAX)
		for sc.Scan() {
			var e Entry
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				continue
			}
			if e.ID == 0 {
				e.ID = j.nextID
			}
			j.nextID = max(j.nextID, e.ID+1)
			j.keep(e)
		}
		f.Close()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, JOURNAL_PERMS)
	if err != nil {
		return nil, err
	}
	j.file, j.enc = f, json.NewEncoder(f)
	return j, nil
}

func (j *Journal) keep(e Entry) {
	j.recent = append(j.recent, e)
	if len(j.recent) > JOURNAL_RECENT {
		j.recent = j.recent[len(j.recent)-JOURNAL_RECENT:]
	}
}

// Append numbers e, writes it and hands it to every subscriber. A slow subscriber misses lines
// rather than blocking the runner; the UI reconnects and backfills by id.
func (j *Journal) Append(e Entry) (Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e.ID = j.nextID
	j.nextID++
	if err := j.enc.Encode(e); err != nil {
		return e, err
	}
	j.keep(e)
	for ch := range j.subs {
		select {
		case ch <- e:
		default:
		}
	}
	return e, nil
}

// Recent returns up to limit entries newer than after, oldest first.
func (j *Journal) Recent(after int64, limit int) []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []Entry{}
	for _, e := range j.recent {
		if e.ID > after {
			out = append(out, e)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// All returns every entry kept in memory, oldest first.
func (j *Journal) All() []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]Entry{}, j.recent...)
}

// Subscribe returns a channel of new entries and a func that ends the subscription.
func (j *Journal) Subscribe() (<-chan Entry, func()) {
	ch := make(chan Entry, SUBSCRIBER_QUEUE)
	j.mu.Lock()
	j.subs[ch] = struct{}{}
	j.mu.Unlock()
	return ch, func() {
		j.mu.Lock()
		delete(j.subs, ch)
		j.mu.Unlock()
	}
}

// Close flushes nothing (every Append is written straight through) and closes the file.
func (j *Journal) Close() error { return j.file.Close() }
