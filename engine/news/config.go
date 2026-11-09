// Package news polls public news, filings, central-bank and macro feeds, tags each item against
// the watchlist, stores it in SQLite and streams new items live to every open client.
package news

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	SOURCES_FILE     = "sources.json"
	WATCHLIST_FILE   = "watchlist.json"
	TYPE_RSS         = "rss"
	TYPE_GDELT       = "gdelt"
	TYPE_POLYMARKET  = "polymarket"
	VENUE_POLYMARKET = "polymarket"
)

//go:embed sources.json watchlist.json
var defaults embed.FS

// Source is one entry in sources.json. MinEvery is a politeness floor under the global poll interval.
type Source struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Type        string        `json:"type"`
	Kind        string        `json:"kind"`
	URL         string        `json:"url"`
	MinEveryRaw string        `json:"min_every,omitempty"`
	MinEvery    time.Duration `json:"-"`
}

// readConfig returns the override file when a path is set, otherwise the embedded default.
func readConfig(path, name string) ([]byte, error) {
	if path != "" {
		return os.ReadFile(path)
	}
	return defaults.ReadFile(name)
}

// LoadSources reads and validates sources.json (embedded unless path is set).
func LoadSources(path string) ([]Source, error) {
	raw, err := readConfig(path, SOURCES_FILE)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Sources []Source `json:"sources"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i := range doc.Sources {
		src := &doc.Sources[i]
		if src.ID == "" || seen[src.ID] {
			return nil, fmt.Errorf("source %d: missing or duplicate id %q", i, src.ID)
		}
		seen[src.ID] = true
		if _, ok := POLLERS[src.Type]; !ok {
			return nil, fmt.Errorf("%s: unknown type %q", src.ID, src.Type)
		}
		if src.Label == "" {
			src.Label = src.ID
		}
		if src.MinEveryRaw == "" {
			continue
		}
		floor, err := time.ParseDuration(src.MinEveryRaw)
		if err != nil {
			return nil, fmt.Errorf("%s: bad min_every %q", src.ID, src.MinEveryRaw)
		}
		src.MinEvery = floor
	}
	return doc.Sources, nil
}
