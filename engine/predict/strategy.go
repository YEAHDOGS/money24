package predict

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// The rulebook: every strategy the desk runs, with its settings and limits, in one JSON file in
	// m24d's data folder. Edit it (or PUT /api/predict/strategies) to add, remove, retune or switch off
	// a strategy; the desk reads it again within a minute, no restart. A strategy is a kind (code that
	// registers itself in a file of its own, like weather.go) plus settings: one kind can run several
	// times with different settings, side by side, each with its own record.
	RULEBOOK_FILE  = "predict-strategies.json"
	RULEBOOK_PERMS = 0o644
	RISK_LOW       = "low"
	RISK_MEDIUM    = "medium"
	RISK_HIGH      = "high"
	MIN_EVERY      = 1 // minutes
	ID_CHARS       = "abcdefghijklmnopqrstuvwxyz0123456789-"
)

// RISK_ORDER ranks the risk labels, so the desk's mode can leave the riskier strategies watching only.
var RISK_ORDER = map[string]int{RISK_LOW: 1, RISK_MEDIUM: 2, RISK_HIGH: 3}

var (
	ErrUnknownKind = errors.New("unknown strategy kind")
	ErrBadRulebook = errors.New("rulebook")
)

// Strategy is one way of finding trades. The desk hands it the markets its scope asks for; it says,
// market by market, which side it would buy and, when it can, the chance it gives YES.
type Strategy interface {
	Scope() Scope
	// Prepare fetches what Evaluate needs, once a pass, for the markets about to be evaluated.
	Prepare(ctx context.Context, env *Env, markets []Market, now time.Time) error
	Evaluate(m Market, now time.Time) (Opinion, bool)
}

// Scope is which markets a strategy looks at: named series, or every market closing soon.
type Scope struct {
	Series      []string // Kalshi series tickers (KXHIGHNY); empty: every market closing within WithinHours
	WithinHours float64
}

// Opinion is a strategy's view of one market.
type Opinion struct {
	Side    string   // the side to buy
	Prob    float64  // the chance YES wins, 0 to 1; 0 when the strategy names a side but no chance
	Reasons []string // why, in a few words each, for the desk
}

// Kind is a strategy's code: how to build one from its settings.
type Kind struct {
	ID    string                                         `json:"id"`
	Name  string                                         `json:"name"`
	About string                                         `json:"about"`
	New   func(params json.RawMessage) (Strategy, error) `json:"-"`
}

var (
	kindsMu sync.RWMutex
	kinds   = map[string]Kind{}
)

// Register adds a kind. Each kind's file calls it from init.
func Register(k Kind) {
	kindsMu.Lock()
	defer kindsMu.Unlock()
	kinds[k.ID] = k
}

// Kinds lists the registered kinds, by id.
func Kinds() []Kind {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	out := make([]Kind, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Entry is what a market must offer before a strategy's opinion becomes a trade.
type Entry struct {
	MinPrice     float64 `json:"min_price"`      // the ask of the side bought, dollars a contract
	MaxPrice     float64 `json:"max_price"`      //
	MaxSpread    float64 `json:"max_spread"`     // that side's ask minus its bid
	MinVolume24h float64 `json:"min_volume_24h"` // contracts traded in the last day
	LeftMinutes  int     `json:"left_minutes"`   // the least time left before trading ends
	MinEdge      float64 `json:"min_edge"`       // chance minus price minus fee, dollars a contract (strategies that give a chance)
	MinProb      float64 `json:"min_prob"`       // the least chance the side bought wins (strategies that give a chance)
	// MaxEdge: an edge bigger than this is more likely the model's mistake than the market's. 0: no cap.
	MaxEdge float64 `json:"max_edge,omitempty"`
}

// Limits cap what one strategy puts in.
type Limits struct {
	StakeUSD      float64 `json:"stake_usd"`       // the most into one market, fee included
	MaxOpen       int     `json:"max_open"`        // open positions
	NewPerPass    int     `json:"new_per_pass"`    //
	DailyStakeUSD float64 `json:"daily_stake_usd"` // into new positions per New York day
}

// Config is one strategy in the rulebook.
type Config struct {
	ID           string          `json:"id"`   // unique: lowercase letters, digits and dashes
	Kind         string          `json:"kind"` // a registered kind
	Name         string          `json:"name"`
	On           bool            `json:"on"`   // off: nothing is read or recorded
	Risk         string          `json:"risk"` // low, medium or high
	EveryMinutes int             `json:"every_minutes"`
	Entry        Entry           `json:"entry"`
	Limits       Limits          `json:"limits"`
	Bands        []float64       `json:"bands,omitempty"`  // price ranges its record is also kept in
	Params       json.RawMessage `json:"params,omitempty"` // the kind's own settings
}

// Rulebook is the desk's whole setup.
type Rulebook struct {
	CapitalUSD   float64 `json:"capital_usd"`    // paper capital for every strategy together
	DailyLossUSD float64 `json:"daily_loss_usd"` // settled losses in a New York day that stop new positions everywhere
	// Mode is the most risk the desk trades: low, medium or high. Strategies above it keep watching
	// and keep their record, but buy nothing.
	Mode       string   `json:"mode"`
	Strategies []Config `json:"strategies"`
}

// running is a strategy built from its config.
type running struct {
	cfg      Config
	strategy Strategy
}

// Build checks the rulebook and builds every strategy in it.
func (rb Rulebook) Build() ([]running, error) {
	if rb.CapitalUSD <= 0 || rb.DailyLossUSD <= 0 {
		return nil, fmt.Errorf("%w: capital_usd and daily_loss_usd must be above 0", ErrBadRulebook)
	}
	if RISK_ORDER[rb.Mode] == 0 {
		return nil, fmt.Errorf("%w: mode %q is not low, medium or high", ErrBadRulebook, rb.Mode)
	}
	seen := map[string]bool{}
	out := []running{}
	for _, c := range rb.Strategies {
		if err := c.check(); err != nil {
			return nil, err
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("%w: two strategies are called %q", ErrBadRulebook, c.ID)
		}
		seen[c.ID] = true
		kindsMu.RLock()
		k, ok := kinds[c.Kind]
		kindsMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("%w %q in %s", ErrUnknownKind, c.Kind, c.ID)
		}
		s, err := k.New(c.Params)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrBadRulebook, c.ID, err)
		}
		out = append(out, running{cfg: c, strategy: s})
	}
	return out, nil
}

func (c Config) check() error {
	bad := func(why string) error { return fmt.Errorf("%w: %s: %s", ErrBadRulebook, c.ID, why) }
	if c.ID == "" || strings.Trim(c.ID, ID_CHARS) != "" {
		return fmt.Errorf("%w: id %q: use lowercase letters, digits and dashes", ErrBadRulebook, c.ID)
	}
	if RISK_ORDER[c.Risk] == 0 {
		return bad("risk must be low, medium or high")
	}
	if c.EveryMinutes < MIN_EVERY {
		return bad("every_minutes must be 1 or more")
	}
	e, l := c.Entry, c.Limits
	if e.MinPrice < 0 || e.MaxPrice > DOLLAR || e.MinPrice >= e.MaxPrice {
		return bad("entry prices must sit between 0 and 1, min under max")
	}
	if l.StakeUSD <= 0 || l.MaxOpen < 0 || l.NewPerPass < 0 || l.DailyStakeUSD < 0 {
		return bad("limits must not be negative, and stake_usd must be above 0")
	}
	for i := 1; i < len(c.Bands); i++ {
		if c.Bands[i] <= c.Bands[i-1] {
			return bad("bands must rise")
		}
	}
	return nil
}

// LoadRulebook reads the rulebook at path. A missing file is written with the defaults first.
func LoadRulebook(path string) (Rulebook, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		rb := DefaultRulebook()
		return rb, SaveRulebook(path, rb)
	}
	if err != nil {
		return Rulebook{}, err
	}
	var rb Rulebook
	if err := json.Unmarshal(body, &rb); err != nil {
		return Rulebook{}, fmt.Errorf("%w: %v", ErrBadRulebook, err)
	}
	return rb, nil
}

// SaveRulebook writes the rulebook to path, readable and indented, through a temporary file.
func SaveRulebook(path string, rb Rulebook) error {
	body, err := json.MarshalIndent(rb, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), RULEBOOK_PERMS); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DefaultRulebook is the setup the desk starts with: every built-in strategy, on, in paper.
func DefaultRulebook() Rulebook {
	return Rulebook{CapitalUSD: DEFAULT_CAPITAL_USD, DailyLossUSD: DEFAULT_DAILY_LOSS_USD, Mode: RISK_HIGH, Strategies: []Config{
		defaultFavorites(),
		defaultWeatherKnown(),
		defaultWeatherForecast(),
	}}
}
