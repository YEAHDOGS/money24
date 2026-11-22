package predict

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	// Favorites near the close: over thousands of markets, contracts priced 85–95¢ in the last hours
	// before they settle have tended to win a little more often than their price says (the
	// favorite–long-shot bias). It needs no data of its own and gives no chance: it names the
	// favorite, and the entry rules decide whether it is cheap enough, liquid enough and close enough.
	KIND_FAVORITES        = "favorites"
	FAVORITES_ID          = "favorites"
	FAVORITES_NAME        = "Favorites near the close"
	FAVORITES_ABOUT       = "Buys the side priced 85–95¢ in the last hours before a market closes, any market, and holds it to the result."
	FAVORITES_REASON      = "favorite at %.0f¢, %s left"
	FAVORITES_WITHIN_HRS  = 6.0
	FAVORITES_MIN_VOLUME  = 200.0
	FAVORITES_STAKE_USD   = 20.0
	FAVORITES_MAX_OPEN    = 25
	FAVORITES_PER_PASS    = 5
	FAVORITES_DAILY_STAKE = 200.0
)

func init() {
	Register(Kind{ID: KIND_FAVORITES, Name: FAVORITES_NAME, About: FAVORITES_ABOUT, New: newFavorites})
}

// FavoritesParams are the favorites kind's own settings.
type FavoritesParams struct {
	WithinHours float64 `json:"within_hours"` // markets whose trading ends within this many hours
}

type favorites struct{ p FavoritesParams }

func newFavorites(raw json.RawMessage) (Strategy, error) {
	p := FavoritesParams{WithinHours: FAVORITES_WITHIN_HRS}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	if p.WithinHours <= 0 {
		return nil, fmt.Errorf("within_hours must be above 0")
	}
	return &favorites{p: p}, nil
}

func (f *favorites) Scope() Scope { return Scope{WithinHours: f.p.WithinHours} }

func (f *favorites) Prepare(context.Context, *Env, []Market, time.Time) error { return nil }

func (f *favorites) Evaluate(m Market, now time.Time) (Opinion, bool) {
	side := SIDE_YES
	if m.NoAsk > m.YesAsk {
		side = SIDE_NO
	}
	left := m.CloseAt.Sub(now).Round(time.Minute)
	return Opinion{Side: side, Reasons: []string{fmt.Sprintf(FAVORITES_REASON, m.Ask(side)*PERCENT, left)}}, true
}

func defaultFavorites() Config {
	params, _ := json.Marshal(FavoritesParams{WithinHours: FAVORITES_WITHIN_HRS})
	return Config{ID: FAVORITES_ID, Kind: KIND_FAVORITES, Name: FAVORITES_NAME, On: true, Risk: RISK_MEDIUM, EveryMinutes: DEFAULT_EVERY_MINUTES,
		Entry:  Entry{MinPrice: DEFAULT_MIN_PRICE, MaxPrice: DEFAULT_MAX_PRICE, MaxSpread: DEFAULT_MAX_SPREAD, MinVolume24h: FAVORITES_MIN_VOLUME, LeftMinutes: DEFAULT_LEFT_MINUTES},
		Limits: Limits{StakeUSD: FAVORITES_STAKE_USD, MaxOpen: FAVORITES_MAX_OPEN, NewPerPass: FAVORITES_PER_PASS, DailyStakeUSD: FAVORITES_DAILY_STAKE},
		Bands:  []float64{0.85, 0.88, 0.91, 0.95}, Params: params}
}
