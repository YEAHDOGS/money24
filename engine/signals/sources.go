package signals

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

// Source types the adapters understand.
const (
	TYPE_X           = "x"
	TYPE_TELEGRAM    = "telegram"
	TYPE_WEBHOOK     = "webhook"
	TYPE_LEADERBOARD = "leaderboard"
	TYPE_EDGAR       = "edgar"
	ROLE_NEWS        = "news"
)

//go:embed default_sources.json
var defaultSources []byte

// Source is one place signals come from. Settings left out of a source fall back to the file defaults.
type Source struct {
	Type     string    `json:"type"`
	Handle   string    `json:"handle,omitempty"`
	Venue    string    `json:"venue,omitempty"`
	Markets  []string  `json:"markets"`
	Role     string    `json:"role,omitempty"`
	Note     string    `json:"note,omitempty"`
	Settings *Settings `json:"settings,omitempty"`
}

// ID is stable across restarts: "x:InvestorsLive", "leaderboard:polymarket", "webhook".
func (s Source) ID() string {
	name := s.Handle
	if name == "" {
		name = s.Venue
	}
	if name == "" {
		return s.Type
	}
	return s.Type + ":" + name
}

// Catalog is the loaded source list.
type Catalog struct {
	Defaults Settings `json:"defaults"`
	GoLive   GoLive   `json:"go_live"`
	Sources  []Source `json:"sources"`
	// CryptoSymbols are tickers priced as coins from any source. A source whose only market is
	// crypto has every symbol priced that way.
	CryptoSymbols []string `json:"crypto_symbols"`
	Blocklist     []Source `json:"blocklist"`
}

// LoadCatalog reads path, or the embedded defaults when path is empty.
func LoadCatalog(path string) (*Catalog, error) {
	raw := defaultSources
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	var c Catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("sources: %w", err)
	}
	blocked := map[string]bool{}
	for _, b := range c.Blocklist {
		blocked[strings.ToLower(b.ID())] = true
	}
	seen := map[string]bool{}
	kept := c.Sources[:0]
	for _, s := range c.Sources {
		id := strings.ToLower(s.ID())
		if blocked[id] {
			return nil, fmt.Errorf("sources: %s is on the blocklist", s.ID())
		}
		if seen[id] {
			return nil, fmt.Errorf("sources: %s listed twice", s.ID())
		}
		seen[id] = true
		kept = append(kept, s)
	}
	c.Sources = kept
	return &c, nil
}

// IsCrypto reports whether a signal should be priced and sized as a coin, not a stock.
func (c *Catalog) IsCrypto(sig Signal) bool {
	if sig.Right != "" {
		return false
	}
	for _, s := range c.Sources {
		if strings.EqualFold(s.ID(), sig.SourceID) && len(s.Markets) == 1 && s.Markets[0] == MARKET_CRYPTO {
			return true
		}
	}
	return slices.Contains(c.CryptoSymbols, sig.Symbol)
}

// SettingsFor returns a source's own settings, or the catalog defaults.
func (c *Catalog) SettingsFor(id string) Settings {
	for _, s := range c.Sources {
		if strings.EqualFold(s.ID(), id) && s.Settings != nil {
			return *s.Settings
		}
	}
	return c.Defaults
}
