package plays

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/internal/diskusage"
)

const (
	PLAYS_FILE = "plays.json"
	LINKS_FILE = "links.json"
	RULES_FILE = "rules.json"
	FILE_PERMS = 0o644
	DIR_PERMS  = 0o755
	ID_BYTES   = 6
	MAX_LINKS  = 2000 // link history kept, newest first
	ID_TIME    = "060102"
)

var ErrNotFound = errors.New("play not found")

// Store keeps plays, the link history and the rulebook as three small JSON files in the data
// folder. Every change rewrites its file through a temp file and a rename, so a crash never
// leaves half a file.
type Store struct {
	dir   string
	mu    sync.Mutex
	plays []Play
	links []Link
	rules Rules
}

func (s *Store) bytesOnDisk() int64 {
	if s == nil {
		return 0
	}
	return diskusage.Files(
		filepath.Join(s.dir, PLAYS_FILE),
		filepath.Join(s.dir, LINKS_FILE),
		filepath.Join(s.dir, RULES_FILE),
	)
}

// OpenStore reads the folder, creating it and the DOGS rulebook on first run.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, DIR_PERMS); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, rules: DOGSRules()}
	for name, into := range map[string]any{PLAYS_FILE: &s.plays, LINKS_FILE: &s.links, RULES_FILE: &s.rules} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, into); err != nil {
			return nil, err
		}
	}
	s.rules = s.rules.merged()
	return s, nil
}

func (s *Store) write(name string, v any) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, FILE_PERMS); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// NewID is a short, sortable, unguessable id: date plus random hex.
func NewID(now time.Time) string {
	buf := make([]byte, ID_BYTES)
	rand.Read(buf)
	return now.UTC().Format(ID_TIME) + "-" + hex.EncodeToString(buf)
}

// Plays returns every play, newest first.
func (s *Store) Plays() []Play {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Play(nil), s.plays...)
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.After(out[b].CreatedAt) })
	return out
}

// Play returns one play by id.
func (s *Store) Play(id string) (Play, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.plays {
		if p.ID == id {
			return p, nil
		}
	}
	return Play{}, ErrNotFound
}

// Save inserts or replaces a play by id.
func (s *Store) Save(p Play) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.plays {
		if s.plays[i].ID == p.ID {
			s.plays[i] = p
			return s.write(PLAYS_FILE, s.plays)
		}
	}
	s.plays = append(s.plays, p)
	return s.write(PLAYS_FILE, s.plays)
}

// Update applies fn to one play under the lock and saves it, so slow work (an AI call) that
// finishes later never overwrites edits made in the meantime.
func (s *Store) Update(id string, fn func(*Play)) (Play, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.plays {
		if s.plays[i].ID == id {
			fn(&s.plays[i])
			return s.plays[i], s.write(PLAYS_FILE, s.plays)
		}
	}
	return Play{}, ErrNotFound
}

// Delete removes a play. Its link history stays.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.plays {
		if s.plays[i].ID == id {
			s.plays = append(s.plays[:i], s.plays[i+1:]...)
			return s.write(PLAYS_FILE, s.plays)
		}
	}
	return ErrNotFound
}

// AddLink records one pasted link at the front of the history.
func (s *Store) AddLink(l Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links = append([]Link{l}, s.links...)
	if len(s.links) > MAX_LINKS {
		s.links = s.links[:MAX_LINKS]
	}
	return s.write(LINKS_FILE, s.links)
}

// Links returns the link history, newest first.
func (s *Store) Links() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Link(nil), s.links...)
}

// Rules returns the rulebook.
func (s *Store) Rules() Rules {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rules
}

// SetRules saves a rulebook, filling anything it leaves out from the defaults.
func (s *Store) SetRules(r Rules) (Rules, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = r.merged()
	return s.rules, s.write(RULES_FILE, s.rules)
}
