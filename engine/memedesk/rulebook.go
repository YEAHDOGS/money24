package memedesk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	RULEBOOK_FILE       = "meme-strategies.json"
	DEFAULT_CAPITAL     = 250.0
	DEFAULT_DAILY_LOSS  = 25.0
	DEFAULT_STAKE       = 10.0
	DEFAULT_MAX_OPEN    = 3
	DEFAULT_NEW_HOUR    = 3
	DEFAULT_DAILY_STAKE = 50.0
	DEFAULT_MIN_LIQ     = 10000.0
	DEFAULT_BUYERS      = 20
	DEFAULT_HOLDER_PCT  = 10.0
	DEFAULT_TOP_PCT     = 40.0
	DEFAULT_LP_PCT      = 90.0
	DEFAULT_SPIKE       = 0.30
	DEFAULT_STOP        = 0.25
	DEFAULT_HOLD_SECS   = 300
	DEFAULT_TP_FIRST    = 0.30
	DEFAULT_TP_SECOND   = 0.60
	DEFAULT_TP_PART     = 0.50
	MAX_PERCENT         = 100.0
	ID_CHARS            = "abcdefghijklmnopqrstuvwxyz0123456789-"
	MAX_RULEBOOK_BYTES  = 256 << 10
	MAX_STRATEGIES      = 32
	MAX_ID_LENGTH       = 64
	FOLLOW_WINDOW       = meme.RECORD_WINDOW
	RULEBOOK_TEMP       = ".meme-rulebook-*"
)

type Entry struct {
	MinLiquidityUSD  float64 `json:"min_liquidity_usd"`
	MinUniqueBuyers  int     `json:"min_unique_buyers"`
	MaxTopHolderPct  float64 `json:"max_top_holder_pct"`
	MaxTopHoldersPct float64 `json:"max_top_holders_pct"`
	MinLPLockedPct   float64 `json:"min_lp_locked_pct"`
	SpikePct         float64 `json:"spike_pct"` // fraction, e.g. 0.30
}

type Limits struct {
	StakeUSD      float64 `json:"stake_usd"`
	MaxOpen       int     `json:"max_open"`
	NewPerHour    int     `json:"new_per_hour"`
	DailyStakeUSD float64 `json:"daily_stake_usd"`
}

type Rung struct {
	Gain     float64 `json:"gain"`
	Fraction float64 `json:"fraction"` // fraction of original units
}

type Exit struct {
	StopPct     float64 `json:"stop_pct"`
	HoldSeconds int     `json:"hold_seconds"`
	TakeProfit  []Rung  `json:"take_profit"`
}

type Config struct {
	ID     string          `json:"id"`
	Kind   string          `json:"kind"`
	On     bool            `json:"on"`
	Risk   string          `json:"risk"`
	Entry  Entry           `json:"entry"`
	Limits Limits          `json:"limits"`
	Exit   Exit            `json:"exit"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Rulebook struct {
	CapitalUSD   float64  `json:"capital_usd"`
	DailyLossUSD float64  `json:"daily_loss_usd"`
	Strategies   []Config `json:"strategies"`
	DefaultsSeen []string `json:"defaults_seen"`
}

func DefaultRulebook() Rulebook {
	rulebook := Rulebook{CapitalUSD: DEFAULT_CAPITAL, DailyLossUSD: DEFAULT_DAILY_LOSS}
	for _, id := range []string{KIND_SHADOW, KIND_ORGANIC, KIND_FADE} {
		cfg := Config{ID: id, Kind: id, On: true, Risk: RISK_HIGH,
			Entry: Entry{MinLiquidityUSD: DEFAULT_MIN_LIQ, MinUniqueBuyers: DEFAULT_BUYERS,
				MaxTopHolderPct: DEFAULT_HOLDER_PCT, MaxTopHoldersPct: DEFAULT_TOP_PCT, MinLPLockedPct: DEFAULT_LP_PCT, SpikePct: DEFAULT_SPIKE},
			Limits: Limits{StakeUSD: DEFAULT_STAKE, MaxOpen: DEFAULT_MAX_OPEN, NewPerHour: DEFAULT_NEW_HOUR, DailyStakeUSD: DEFAULT_DAILY_STAKE},
			Exit: Exit{StopPct: DEFAULT_STOP, HoldSeconds: DEFAULT_HOLD_SECS,
				TakeProfit: []Rung{{Gain: DEFAULT_TP_FIRST, Fraction: DEFAULT_TP_PART}, {Gain: DEFAULT_TP_SECOND, Fraction: DEFAULT_TP_PART}}},
		}
		rulebook.Strategies = append(rulebook.Strategies, cfg)
		rulebook.DefaultsSeen = append(rulebook.DefaultsSeen, id)
	}
	return rulebook
}

func (rb *Rulebook) offerDefaults() bool {
	seen, have := map[string]bool{}, map[string]bool{}
	for _, id := range rb.DefaultsSeen {
		seen[id] = true
	}
	for _, cfg := range rb.Strategies {
		have[cfg.ID] = true
	}
	changed := false
	for _, cfg := range DefaultRulebook().Strategies {
		if seen[cfg.ID] {
			continue
		}
		rb.DefaultsSeen = append(rb.DefaultsSeen, cfg.ID)
		changed = true
		if !have[cfg.ID] {
			rb.Strategies = append(rb.Strategies, cfg)
		}
	}
	return changed
}

func (rb Rulebook) build() ([]configured, error) {
	if !positive(rb.CapitalUSD) || !positive(rb.DailyLossUSD) {
		return nil, errors.New("paper capital and daily loss threshold must be positive")
	}
	if len(rb.Strategies) > MAX_STRATEGIES {
		return nil, errors.New("too many strategies")
	}
	seen := map[string]bool{}
	rules := make([]configured, 0, len(rb.Strategies))
	for _, cfg := range rb.Strategies {
		if cfg.ID == "" || len(cfg.ID) > MAX_ID_LENGTH || strings.Trim(cfg.ID, ID_CHARS) != "" || seen[cfg.ID] {
			return nil, fmt.Errorf("invalid or duplicate strategy ID %q", cfg.ID)
		}
		seen[cfg.ID] = true
		if err := cfg.check(); err != nil {
			return nil, fmt.Errorf("%s: %w", cfg.ID, err)
		}
		kindsMu.RLock()
		kind, ok := kinds[cfg.Kind]
		kindsMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("%s: unknown strategy kind %q", cfg.ID, cfg.Kind)
		}
		rule, err := kind.New(cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cfg.ID, err)
		}
		rules = append(rules, configured{config: cfg, rule: rule, buy: kind.Buy})
	}
	return rules, nil
}

func (c Config) check() error {
	switch c.Risk {
	case RISK_LOW, RISK_MEDIUM, RISK_HIGH:
	default:
		return errors.New("invalid risk label")
	}
	l, e, x := c.Limits, c.Entry, c.Exit
	if !positive(l.StakeUSD) || !positive(l.DailyStakeUSD) || l.MaxOpen < 1 || l.NewPerHour < 1 {
		return errors.New("strategy limits must be positive")
	}
	if e.MinLiquidityUSD < 0 || e.MinUniqueBuyers < 1 || e.MaxTopHolderPct <= 0 ||
		e.MaxTopHolderPct > e.MaxTopHoldersPct || e.MaxTopHoldersPct > MAX_PERCENT ||
		e.MinLPLockedPct < 0 || e.MinLPLockedPct > MAX_PERCENT || !positive(e.SpikePct) {
		return errors.New("invalid entry rules")
	}
	if x.HoldSeconds < 1 || x.HoldSeconds > int(FOLLOW_WINDOW/time.Second) ||
		x.StopPct <= 0 || x.StopPct >= 1 {
		return errors.New("invalid stop or holding time")
	}
	var last, sum float64
	for _, rung := range x.TakeProfit {
		sum += rung.Fraction
		if rung.Gain <= last || !positive(rung.Fraction) || sum > 1+FRACTION_EPSILON {
			return errors.New("take-profit targets must rise and sell at most the original position")
		}
		last = rung.Gain
	}
	return nil
}

func decodeRulebook(raw io.Reader) (Rulebook, error) {
	var rb Rulebook
	data, err := io.ReadAll(io.LimitReader(raw, MAX_RULEBOOK_BYTES+1))
	if err != nil {
		return rb, err
	}
	if len(data) > MAX_RULEBOOK_BYTES {
		return rb, errors.New("rulebook too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rb); err != nil {
		return rb, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return rb, errors.New("expected one rulebook object")
	}
	return rb, nil
}

func readRulebook(path string) (Rulebook, bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultRulebook(), true, nil
	}
	if err != nil {
		return Rulebook{}, false, err
	}
	defer file.Close()
	rb, err := decodeRulebook(file)
	if err != nil {
		return rb, false, err
	}
	changed := rb.offerDefaults()
	return rb, changed, nil
}

func saveRulebook(path string, rb Rulebook) error {
	file, err := os.CreateTemp(filepath.Dir(path), RULEBOOK_TEMP)
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	err = enc.Encode(rb)
	closeErr := file.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
