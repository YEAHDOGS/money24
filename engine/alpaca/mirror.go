package alpaca

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/signals"
)

const (
	// The mirror places every desk trade at Alpaca as it is journaled: the entry as a limit buy at the
	// desk's own price, each exit as a market sell of what the broker really holds. The desk's journal
	// stays the record of decisions; Alpaca's fills say what the market really gave, so the two can be
	// compared trade by trade. Paper only: FromEnv refuses live keys.
	CLIENT_PREFIX   = "m24-"
	INSTALL_BYTES   = 3 // a random tag per install in every order id, so a fresh journal never reuses one
	ENTRY_SUFFIX    = "-in"
	QUEUE           = 1024
	ACTIONS_KEPT    = 60
	RECONCILE_EVERY = time.Minute
	ORDER_TIMEOUT   = 20 * time.Second
	STATE_PERMS     = 0o600
	ORDERS_SHOWN    = 20
	PENNY_UNDER     = 3.0  // options under $3 trade in pennies, at or over it in nickels...
	NICKEL          = 0.05 // ...except the penny names, which trade in pennies at every price
	PENNY           = 0.01
	COIN_DIGITS     = 6 // significant digits for a coin under a dollar
	CENTS           = 100
	OCC_DATE        = "060102"
	EXPIRY_DATE     = "2006-01-02"
	OCC_STRIKE      = 1000
	ACTION_FILL     = "fill"
	ACTION_CLOSE    = "close"
	ACTION_VOID     = "void"
	STATUS_NEW      = "new"
	STATUS_PARTIAL  = "partially_filled"
	STATUS_ACCEPTED = "accepted"
	STATUS_PENDING  = "pending_new"
	NOTE_DUPE       = "already placed"
	NOTE_NOTHING    = "nothing held at the broker to sell"
	NOTE_CANCELLED  = "entry cancelled: the desk closed before it filled"
	NOTE_SKIP_CHAIN = "on-chain tokens do not trade at Alpaca"
)

// PENNY_ROOTS trade options in pennies at every price.
var PENNY_ROOTS = map[string]bool{"SPY": true, "QQQ": true, "IWM": true}

// Action is one thing the mirror did, or tried to.
type Action struct {
	At       time.Time `json:"at"`
	PlanID   int64     `json:"plan_id"`
	LineID   int64     `json:"line_id"`
	Symbol   string    `json:"symbol"`
	Side     string    `json:"side"`
	Qty      string    `json:"qty,omitempty"`
	Limit    string    `json:"limit,omitempty"`
	Desk     float64   `json:"desk_price,omitempty"` // the price the desk's own book used, to set beside the broker's fill
	ClientID string    `json:"client_id,omitempty"`
	Status   string    `json:"status,omitempty"`
	Note     string    `json:"note,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Status is the broker side of the desk, for the page.
type Status struct {
	Mode      string     `json:"mode"`
	Account   *Account   `json:"account,omitempty"`
	Positions []Position `json:"positions"`
	Orders    []Order    `json:"orders"`
	Actions   []Action   `json:"actions"` // newest first
	CheckedAt time.Time  `json:"checked_at,omitzero"`
	Error     string     `json:"error,omitempty"`
}

// Mirror places the journal's trades at Alpaca.
type Mirror struct {
	Client    *Client
	Journal   *signals.Journal
	StateFile string // remembers the last journal line handled, so a restart neither repeats nor misses

	mu      sync.Mutex
	status  Status
	last    int64
	install string
}

// mirrorState is what the mirror remembers across restarts.
type mirrorState struct {
	LastLine int64  `json:"last_line"`
	Install  string `json:"install"`
}

// clientID is the order id for one journal line, unique to this install: m24-<install>-<line>[-in].
func (m *Mirror) clientID(line int64, suffix string) string {
	return fmt.Sprintf("%s%s-%d%s", CLIENT_PREFIX, m.install, line, suffix)
}

// symbolFor is Alpaca's name for what a signal trades, and whether it is a coin.
func symbolFor(sig signals.Signal) (symbol string, coin bool, err error) {
	switch {
	case sig.Address != "":
		return "", false, errors.New(NOTE_SKIP_CHAIN)
	case sig.Right != "":
		day, err := time.Parse(EXPIRY_DATE, sig.Expiry)
		if err != nil {
			return "", false, err
		}
		return fmt.Sprintf("%s%s%s%08d", strings.ToUpper(sig.Symbol), day.Format(OCC_DATE), sig.Right, int64(math.Round(sig.Strike*OCC_STRIKE))), false, nil
	case sig.Market == signals.MARKET_CRYPTO:
		return CryptoPair(sig.Symbol), true, nil
	}
	return strings.ToUpper(sig.Symbol), false, nil
}

// limitFor rounds a buy limit up to a price the venue accepts, so it is never below the desk's price.
func limitFor(sig signals.Signal, price float64, coin bool) string {
	switch {
	case coin && price < 1:
		digits := COIN_DIGITS - 1 - int(math.Floor(math.Log10(price)))
		return strconv.FormatFloat(price, 'f', max(digits, 0), 64)
	case sig.Right != "" && price >= PENNY_UNDER && !PENNY_ROOTS[strings.ToUpper(sig.Symbol)]:
		return strconv.FormatFloat(math.Ceil(price/NICKEL-1e-9)*NICKEL, 'f', 2, 64)
	}
	return strconv.FormatFloat(math.Ceil(price*CENTS-1e-9)/CENTS, 'f', 2, 64)
}

func (m *Mirror) note(a Action) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Actions = append([]Action{a}, m.status.Actions...)
	if len(m.status.Actions) > ACTIONS_KEPT {
		m.status.Actions = m.status.Actions[:ACTIONS_KEPT]
	}
	if a.Error != "" {
		log.Printf("alpaca: %s %s %s: %s", a.Side, a.Qty, a.Symbol, a.Error)
		return
	}
	log.Printf("alpaca: %s %s %s %s %s", a.Side, a.Qty, a.Symbol, a.Status, a.Note)
}

// enter places the desk's entry as a limit buy at its own price.
func (m *Mirror) enter(ctx context.Context, e signals.Entry) {
	a := Action{At: time.Now().UTC(), PlanID: e.ID, LineID: e.ID, Side: SIDE_BUY, ClientID: m.clientID(e.ID, ENTRY_SUFFIX)}
	symbol, coin, err := symbolFor(e.Signal)
	a.Symbol = symbol
	if err != nil {
		a.Note = err.Error()
		m.note(a)
		return
	}
	tif := TIF_DAY
	if coin {
		tif = TIF_GTC
	}
	a.Qty, a.Limit, a.Desk = strconv.FormatFloat(e.Plan.Qty, 'f', -1, 64), limitFor(e.Signal, e.Plan.Entry, coin), e.Plan.Entry
	o, err := m.Client.Place(ctx, Order{ClientOrderID: a.ClientID, Symbol: symbol, Qty: a.Qty, Side: SIDE_BUY, Type: TYPE_LIMIT, TimeInForce: tif, LimitPrice: a.Limit})
	switch {
	case errors.Is(err, ErrDupe):
		a.Note = NOTE_DUPE
	case err != nil:
		a.Error = err.Error()
	default:
		a.Status = o.Status
	}
	m.note(a)
}

// held is how much of a symbol the broker really holds. Coin positions drop the slash (BTCUSD).
func (m *Mirror) held(ctx context.Context, symbol string) (float64, error) {
	positions, err := m.Client.Positions(ctx)
	if err != nil {
		return 0, err
	}
	want := strings.ReplaceAll(symbol, "/", "")
	for _, p := range positions {
		if strings.ReplaceAll(p.Symbol, "/", "") == want {
			return p.Qty, nil
		}
	}
	return 0, nil
}

// exit sells what the desk sold, never more than the broker holds. When the desk closes a trade whose
// entry never filled at the broker, the entry is cancelled.
func (m *Mirror) exit(ctx context.Context, e signals.Entry) {
	a := Action{At: time.Now().UTC(), PlanID: e.PlanID, LineID: e.ID, Side: SIDE_SELL, ClientID: m.clientID(e.ID, "")}
	symbol, coin, err := symbolFor(e.Signal)
	a.Symbol = symbol
	if err != nil {
		a.Note = err.Error()
		m.note(a)
		return
	}
	closing := e.Action == ACTION_CLOSE || e.Action == ACTION_VOID
	if e.Result != nil {
		a.Desk = e.Result.Price
	}
	if entry, err := m.Client.ByClientID(ctx, m.clientID(e.PlanID, ENTRY_SUFFIX)); err == nil && closing {
		switch entry.Status {
		case STATUS_NEW, STATUS_PARTIAL, STATUS_ACCEPTED, STATUS_PENDING:
			if err := m.Client.Cancel(ctx, entry.ID); err == nil {
				a.Note = NOTE_CANCELLED
			}
		}
	}
	have, err := m.held(ctx, symbol)
	if err != nil {
		a.Error = err.Error()
		m.note(a)
		return
	}
	want := have
	if e.Result != nil && e.Result.Qty > 0 && e.Action == ACTION_FILL {
		want = math.Min(have, e.Result.Qty)
	}
	if want <= 0 {
		a.Note = strings.TrimPrefix(a.Note+"; "+NOTE_NOTHING, "; ")
		m.note(a)
		return
	}
	tif := TIF_DAY
	if coin {
		tif = TIF_GTC
	}
	a.Qty = strconv.FormatFloat(want, 'f', -1, 64)
	o, err := m.Client.Place(ctx, Order{ClientOrderID: a.ClientID, Symbol: symbol, Qty: a.Qty, Side: SIDE_SELL, Type: TYPE_MARKET, TimeInForce: tif})
	switch {
	case errors.Is(err, ErrDupe):
		a.Note = NOTE_DUPE
	case err != nil:
		a.Error = err.Error()
	default:
		a.Status = o.Status
	}
	m.note(a)
}

// handle acts on one journal line and remembers it was handled.
func (m *Mirror) handle(e signals.Entry) {
	if e.ID <= m.last {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), ORDER_TIMEOUT)
	defer cancel()
	switch {
	case e.Plan != nil:
		m.enter(ctx, e)
	case e.PlanID != 0 && (e.Action == ACTION_FILL || e.Action == ACTION_CLOSE || e.Action == ACTION_VOID):
		m.exit(ctx, e)
	}
	m.last = e.ID
	m.save()
}

// save writes what the mirror remembers.
func (m *Mirror) save() {
	if b, err := json.Marshal(mirrorState{LastLine: m.last, Install: m.install}); err == nil {
		os.WriteFile(m.StateFile, b, STATE_PERMS)
	}
}

// reconcile reads the broker's account, positions and recent orders for the page.
func (m *Mirror) reconcile(ctx context.Context) {
	st := Status{Mode: m.Client.Mode, CheckedAt: time.Now().UTC(), Positions: []Position{}, Orders: []Order{}}
	a, err := m.Client.Account(ctx)
	if err == nil {
		st.Account = &a
		st.Positions, err = m.Client.Positions(ctx)
	}
	if err == nil {
		st.Orders, err = m.Client.Orders(ctx, "all", ORDERS_SHOWN)
	}
	if err != nil {
		st.Error = err.Error()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st.Actions = m.status.Actions
	m.status = st
}

// Status is the broker side right now.
func (m *Mirror) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.status
	if st.Actions == nil {
		st.Actions = []Action{}
	}
	return st
}

// Run mirrors the journal until ctx ends. On a first run it starts from the journal's newest line, so
// trades from before the broker was connected are never sent; after that it resumes where it stopped.
func (m *Mirror) Run(ctx context.Context) {
	lines, done := m.Journal.Subscribe()
	defer done()
	queue := make(chan signals.Entry, QUEUE)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-lines:
				queue <- e
			}
		}
	}()
	var saved mirrorState
	recent := m.Journal.Recent(0, signals.JOURNAL_RECENT)
	b, err := os.ReadFile(m.StateFile)
	known := err == nil && json.Unmarshal(b, &saved) == nil && saved.Install != ""
	m.last, m.install = saved.LastLine, saved.Install
	if !known {
		tag := make([]byte, INSTALL_BYTES)
		rand.Read(tag)
		m.install = hex.EncodeToString(tag)
		if len(recent) > 0 {
			m.last = recent[len(recent)-1].ID
		}
		m.save()
	}
	for _, e := range recent {
		m.handle(e) // what was journaled while the runner was down; already handled lines are skipped
	}
	check := time.NewTicker(RECONCILE_EVERY)
	defer check.Stop()
	m.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-queue:
			m.handle(e)
		case <-check.C:
			m.reconcile(ctx)
		}
	}
}
