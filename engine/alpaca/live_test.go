package alpaca

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"
)

// loads ../../.env into the environment for a live check; never commits anything.
func loadEnv(t *testing.T) {
	f, err := os.Open("../../.env")
	if err != nil {
		t.Skip("no .env")
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		os.Setenv(strings.TrimSpace(k), strings.TrimSpace(v))
	}
}

func TestLiveAccount(t *testing.T) {
	if os.Getenv("M24D_LIVE") == "" {
		t.Skip()
	}
	loadEnv(t)
	c, err := FromEnv()
	if err != nil || c == nil {
		t.Fatalf("client: %v", err)
	}
	ctx := context.Background()
	a, err := c.Account(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("mode %s | status %s | cash $%.2f | equity $%.2f | buying power $%.2f | non-margin bp $%.2f | multiplier %s | crypto %s | options approved %d, level %d | PDT %v | blocked %v",
		c.Mode, a.Status, a.Cash, a.Equity, a.BuyingPower, a.NonMarginBP, a.Multiplier, a.CryptoStatus, a.OptionsApproved, a.OptionsLevel, a.PatternDay, a.TradingBlocked)
	q, err := c.LatestCrypto(ctx, "BTC/USD")
	t.Logf("BTC/USD bid %.2f ask %.2f err %v", q.Bid, q.Ask, err)
	pos, err := c.Positions(ctx)
	t.Logf("positions %d err %v", len(pos), err)
	orders, err := c.Orders(ctx, "all", 5)
	t.Logf("recent orders %d err %v", len(orders), err)
}

func TestLiveCashOnly(t *testing.T) {
	if os.Getenv("M24D_LIVE_CASH") == "" {
		t.Skip()
	}
	loadEnv(t)
	c, _ := FromEnv()
	cfg, err := c.CashOnly(context.Background())
	t.Logf("config now: margin multiplier %q, no shorting %v, err %v", cfg.MaxMargin, cfg.NoShorting, err)
	a, _ := c.Account(context.Background())
	t.Logf("account: multiplier %s, buying power $%.2f", a.Multiplier, a.BuyingPower)
}
