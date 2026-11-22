package predict

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSettleWaitsForFinalResultAndPaysScalar(t *testing.T) {
	for _, side := range []string{SIDE_YES, SIDE_NO} {
		t.Run(side, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "book.jsonl")
			book, err := OpenBook(path, 100)
			if err != nil {
				t.Fatal(err)
			}
			defer book.Close()
			for _, line := range []Line{
				{At: testNow, Kind: KIND_WATCH, Side: side, Ticker: "T", Price: 0.8},
				{At: testNow, Kind: KIND_BUY, Side: side, Ticker: "T", Qty: 10, CostUSD: 8, FeeUSD: 0.12},
			} {
				if _, err := book.Append(line); err != nil {
					t.Fatal(err)
				}
			}
			for _, tc := range []struct {
				status, result, settlement string
				want                       int
			}{
				{"determined", "yes", "1.0000", 0},
				{"disputed", "yes", "1.0000", 0},
				{"amended", "no", "0.0000", 0},
				{"finalized", "scalar", "", 0},
				{"finalized", "scalar", "invalid", 0},
				{"finalized", "scalar", "1.5000", 0},
				{"finalized", "scalar", "0.6500", 1},
			} {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					fmt.Fprintf(w, `{"markets":[{"ticker":"T","status":%q,"result":%q,"settlement_value_dollars":%q}]}`, tc.status, tc.result, tc.settlement)
				}))
				d := &Desk{Book: book, Kalshi: &Kalshi{HTTP: srv.Client(), Base: srv.URL}}
				n, err := d.settle(context.Background(), testNow)
				srv.Close()
				if err != nil || n != tc.want {
					t.Errorf("%s result=%s value=%q: settled %d, want %d, error %v", tc.status, tc.result, tc.settlement, n, tc.want, err)
				}
			}
			want := 6.5
			if side == SIDE_NO {
				want = 3.5
			}
			replayed, err := OpenBook(path, 100)
			if err != nil {
				t.Fatal(err)
			}
			defer replayed.Close()
			holding, account := replayed.Holding("T"), replayed.Account("")
			if !holding.Settled || holding.PayoutUSD != want || account.CashUSD != round(100-8.12+want, USD_DIGITS) {
				t.Errorf("replayed holding %+v, account %+v, want payout %v", holding, account, want)
			}
		})
	}
}

func TestBinaryPayoutWithoutValidSettlementValue(t *testing.T) {
	for _, value := range []string{"", " ", "invalid", "NaN", "1.5000"} {
		m := (wireMarket{Status: "finalized", Result: RESULT_YES, Settlement: value}).market()
		if !m.Finished() || m.Payout(SIDE_YES) != DOLLAR || m.Payout(SIDE_NO) != 0 {
			t.Errorf("value %q: got %+v", value, m)
		}
	}
}
