package predict

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLivePass runs one real pass against Kalshi's public data into a throwaway book: M24D_LIVE=1 go test ./predict -run Live -v
func TestLivePass(t *testing.T) {
	if os.Getenv("M24D_LIVE") == "" {
		t.Skip("set M24D_LIVE=1 to read Kalshi")
	}
	book, err := OpenBook(filepath.Join(t.TempDir(), "book.jsonl"), DEFAULT_CAPITAL_USD)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	d := &Desk{Book: book, Kalshi: &Kalshi{HTTP: &http.Client{Timeout: 30 * time.Second}}, Rules: DefaultRules()}
	ctx, cancel := context.WithTimeout(context.Background(), PASS_TIMEOUT)
	defer cancel()
	if err := d.pass(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	s := d.Status()
	t.Logf("state %s %s · pass %+v · account %+v", s.State, s.Why, s.Pass, s.Account)
	for _, p := range s.Open {
		t.Logf("open: %.0f %s %s @ %.4f $%.2f+%.2f closes %s · %s", p.Buy.Qty, p.Buy.Side, p.Buy.Ticker, p.Buy.Price, p.Buy.CostUSD, p.Buy.FeeUSD, p.Buy.CloseAt.Format(time.RFC3339), p.Buy.Title)
	}
}
