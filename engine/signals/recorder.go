package signals

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

const POSTS_PERMS = 0o644

// RawPost is one post exactly as it arrived, before any parsing. Posts get deleted and edited, and
// the parser changes; this file is the only honest history to backtest and re-parse against.
type RawPost struct {
	SeenAt   time.Time `json:"seen_at"`
	PostedAt time.Time `json:"posted_at,omitzero"`
	SourceID string    `json:"source_id"`
	PostID   string    `json:"post_id,omitempty"`
	ThreadID string    `json:"thread_id,omitempty"`
	URL      string    `json:"url,omitempty"`
	Manual   bool      `json:"manual,omitempty"`
	Text     string    `json:"text"`
}

// Recorder appends every post to a JSONL file. An edited post arrives again and is kept again,
// so both versions stay on record.
type Recorder struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

func OpenRecorder(path string) (*Recorder, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, POSTS_PERMS)
	if err != nil {
		return nil, err
	}
	return &Recorder{file: f, enc: json.NewEncoder(f)}, nil
}

// Record keeps the post behind sig, whatever the parser made of it.
func (r *Recorder) Record(sig Signal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enc.Encode(RawPost{SeenAt: sig.SeenAt.UTC(), PostedAt: sig.PostedAt, SourceID: sig.SourceID,
		PostID: sig.PostID, ThreadID: sig.ThreadID, URL: sig.URL, Manual: sig.Manual, Text: sig.Raw})
}

func (r *Recorder) Close() error { return r.file.Close() }

// Replay re-parses every recorded post with today's parser and calls each with the result.
// It is how a parser change is measured against real history instead of guesses.
func Replay(path string, each func(RawPost, Signal)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, JOURNAL_LINE_MAX), JOURNAL_LINE_MAX)
	for sc.Scan() {
		var p RawPost
		if json.Unmarshal(sc.Bytes(), &p) != nil {
			continue
		}
		// Expiries without a year resolve against the post's own date, as they did live.
		ref := p.PostedAt
		if ref.IsZero() {
			ref = p.SeenAt
		}
		sig := Parse(p.Text, ref)
		sig.SourceID, sig.PostID, sig.ThreadID, sig.URL, sig.Manual = p.SourceID, p.PostID, p.ThreadID, p.URL, p.Manual
		sig.SeenAt, sig.PostedAt = p.SeenAt, p.PostedAt
		each(p, sig)
	}
	return sc.Err()
}
