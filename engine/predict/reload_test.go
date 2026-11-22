package predict

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReloadDoesNotMarkUnreadRulebookAsLoaded(t *testing.T) {
	const kindID = "test-reload"
	path := filepath.Join(t.TempDir(), RULEBOOK_FILE)
	book, err := OpenBook(filepath.Join(t.TempDir(), "book.jsonl"), 100)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	first := DefaultRulebook()
	first.Strategies = first.Strategies[:1]
	first.Strategies[0].Kind = kindID
	second := first
	second.CapitalUSD = 200
	replace := true
	Register(Kind{ID: kindID, New: func(params json.RawMessage) (Strategy, error) {
		if replace {
			replace = false
			if err := SaveRulebook(path, second); err != nil {
				return nil, err
			}
			mod := testNow.Add(time.Hour)
			if err := os.Chtimes(path, mod, mod); err != nil {
				return nil, err
			}
		}
		return newFavorites(params)
	}})
	t.Cleanup(func() {
		kindsMu.Lock()
		delete(kinds, kindID)
		kindsMu.Unlock()
	})
	if err := SaveRulebook(path, first); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, testNow, testNow); err != nil {
		t.Fatal(err)
	}
	d := &Desk{Book: book, RulebookPath: path}
	for range 2 {
		if err := d.Reload(); err != nil {
			t.Fatal(err)
		}
	}
	if got := d.Status().CapitalUSD; got != second.CapitalUSD {
		t.Fatalf("loaded capital $%.2f, want $%.2f from replacement rulebook", got, second.CapitalUSD)
	}
}

func TestBuyChecksDailyStakeOnFillDay(t *testing.T) {
	book, err := OpenBook(filepath.Join(t.TempDir(), "book.jsonl"), 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	c := defaultFavorites()
	now := time.Now()
	if _, err := book.Append(Line{At: now, Kind: KIND_BUY, Strategy: c.ID, Ticker: "EARLIER", Event: "OTHER", CostUSD: c.Limits.DailyStakeUSD}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"orderbook_fp":{"no_dollars":[["0.1000","25.00"]]}}`)
	}))
	defer srv.Close()
	d := &Desk{Book: book, rulebook: DefaultRulebook(), Kalshi: &Kalshi{HTTP: srv.Client(), Base: srv.URL}}
	pick := candidate{market: Market{Ticker: "T", Event: "E", CloseAt: now.Add(time.Hour)}, side: SIDE_YES, ask: 0.9}
	r := build(t, c)
	pass, _, _, err := d.runOne(t.Context(), r, []Market{
		{Ticker: pick.market.Ticker, Event: pick.market.Event, CloseAt: pick.market.CloseAt,
			Type: MARKET_BINARY, YesAsk: pick.ask, YesBid: 0.89, NoAsk: 0.11, Volume24h: 500},
	}, now.In(newYork).AddDate(0, 0, -1), d.ruleVersion)
	n := pass.Bought
	if err != nil || n != 0 || book.Holding("T") != nil {
		t.Fatalf("bought %d after the fill-day stake cap was used, error %v", n, err)
	}
}

func TestRulebookPUTDisablesBuyAlreadyFetchingAsks(t *testing.T) {
	dir := t.TempDir()
	book, err := OpenBook(filepath.Join(dir, "book.jsonl"), 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	now := time.Now()
	asked, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/markets/T/orderbook" {
			close(asked)
			<-release
			fmt.Fprint(w, `{"orderbook_fp":{"no_dollars":[["0.1000","25.00"]]}}`)
			return
		}
		fmt.Fprintf(w, `{"markets":[{"ticker":"T","event_ticker":"E","market_type":"binary","status":"active","close_time":%q,"yes_ask_dollars":"0.9000","yes_bid_dollars":"0.8900","no_ask_dollars":"0.1100","volume_24h_fp":"500.00"}]}`,
			now.Add(time.Hour).Format(time.RFC3339))
	}))
	defer srv.Close()
	d := &Desk{Book: book, RulebookPath: filepath.Join(dir, RULEBOOK_FILE), Kalshi: &Kalshi{HTTP: srv.Client(), Base: srv.URL}, settledAt: now}
	rb := DefaultRulebook()
	rb.Strategies = rb.Strategies[:1]
	if err := SaveRulebook(d.RulebookPath, rb); err != nil {
		t.Fatal(err)
	}
	if err := d.Reload(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.tick(t.Context(), now)
	}()
	select {
	case <-asked:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("pass did not request the order book")
	}
	rb.Strategies[0].On = false
	body, err := json.Marshal(rb)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	d.putRulebook(res, httptest.NewRequest(http.MethodPut, "/api/predict/strategies", bytes.NewReader(body)))
	close(release)
	<-done
	if res.Code != http.StatusOK {
		t.Fatalf("PUT returned %d: %s", res.Code, res.Body.String())
	}
	status := d.Status()
	if s := status.Strategies[0]; s.Config.On || s.State != STATE_OFF || status.Account.Open != 0 {
		t.Fatalf("disabled rule on=%v state=%s, open positions=%d", s.Config.On, s.State, status.Account.Open)
	}
}
