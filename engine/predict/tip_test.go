package predict

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuyTipFillsFromTheBookWithinDeskLimits(t *testing.T) {
	closeAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	status := STATUS_ACTIVE
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/orderbook") {
			fmt.Fprint(w, `{"orderbook_fp":{"yes_dollars":[["0.6000","500.00"]]}}`)
			return
		}
		fmt.Fprintf(w, `{"markets":[{"ticker":"T","event_ticker":"E","market_type":"binary","title":"Question","status":%q,"close_time":%q,"no_ask_dollars":"0.40","no_bid_dollars":"0.38"}]}`, status, closeAt)
	}))
	defer srv.Close()
	book, err := OpenBook(filepath.Join(t.TempDir(), "book.jsonl"), 100)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	d := &Desk{Book: book, rulebook: DefaultRulebook(), Kalshi: &Kalshi{HTTP: srv.Client(), Base: srv.URL}}
	status = STATUS_SETTLED
	if _, err := d.BuyTip(t.Context(), "T", SIDE_NO, ""); !errors.Is(err, ErrTipNoMarket) {
		t.Fatalf("bought a finished market: %v", err)
	}
	status = STATUS_ACTIVE
	d.off = true
	if _, err := d.BuyTip(t.Context(), "T", SIDE_NO, ""); !errors.Is(err, ErrTipDeskOff) {
		t.Fatalf("bought with the desk off: %v", err)
	}
	d.off = false
	line, err := d.BuyTip(t.Context(), "T", SIDE_NO, "a tip")
	if err != nil {
		t.Fatal(err)
	}
	if line.Strategy != TIP_STRATEGY || line.Side != SIDE_NO || line.Price != 0.4 || line.Qty <= 0 || line.CostUSD+line.FeeUSD > TIP_STAKE_USD || !strings.Contains(line.Note, "a tip") {
		t.Fatalf("unexpected fill: %+v", line)
	}
	if _, err := d.BuyTip(t.Context(), "T", SIDE_NO, ""); !errors.Is(err, ErrTipHeld) {
		t.Fatalf("bought the same market twice: %v", err)
	}
	if account := d.Book.Account(""); account.Open != 1 || account.OpenUSD > TIP_STAKE_USD {
		t.Fatalf("book does not hold one stake: %+v", account)
	}
}
