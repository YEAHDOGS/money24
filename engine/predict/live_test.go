package predict

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLivePass runs every default strategy once against real Kalshi prices and real weather data, into
// a throwaway book and rulebook: M24D_LIVE=1 go test ./predict -run Live -v
func TestLivePass(t *testing.T) {
	if os.Getenv("M24D_LIVE") == "" {
		t.Skip("set M24D_LIVE=1 to read Kalshi")
	}
	dir := t.TempDir()
	book, err := OpenBook(filepath.Join(dir, "book.jsonl"), DEFAULT_CAPITAL_USD)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	client := &http.Client{Timeout: 30 * time.Second}
	d := &Desk{Book: book, Kalshi: &Kalshi{HTTP: client}, Env: &Env{HTTP: client}, RulebookPath: filepath.Join(dir, RULEBOOK_FILE)}
	if err := d.Reload(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), PASS_TIMEOUT)
	defer cancel()
	d.tick(ctx, time.Now())
	s := d.Status()
	for _, st := range s.Strategies {
		t.Logf("%s: %s %s · pass %+v · error %q", st.Config.ID, st.State, st.Why, st.Pass, st.LastError)
	}
	t.Logf("account %+v", s.Account)
	for _, p := range s.Open {
		t.Logf("open [%s]: %.0f %s %s @ %.4f $%.2f+%.2f · %s · %s", p.Buy.Strategy, p.Buy.Qty, p.Buy.Side, p.Buy.Ticker, p.Buy.Price, p.Buy.CostUSD, p.Buy.FeeUSD, p.Buy.Title, p.Buy.Note)
	}
}
