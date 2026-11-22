package predict

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/coinbase"
)

func TestPriceProbAddsUp(t *testing.T) {
	const price, spread = 60000.0, 0.002
	above := priceProb(price, 60000, math.Inf(1), spread)
	if math.Abs(above-0.5) > 1e-9 {
		t.Fatalf("at the money got %.4f, want 0.5", above)
	}
	parts := priceProb(price, 0, 59900, spread) + priceProb(price, 59900, 60100, spread) + priceProb(price, 60100, math.Inf(1), spread)
	if math.Abs(parts-1) > 1e-9 {
		t.Fatalf("ranges covering everything add to %.6f", parts)
	}
	if p := priceProb(price, 61000, math.Inf(1), spread); p > 1e-6 {
		t.Fatalf("a 1.7%% jump with a 0.2%% spread got %.8f", p)
	}
}

func TestMinuteVolSkipsGaps(t *testing.T) {
	candles := []coinbase.Candle{}
	price := 100.0
	for i := range 61 {
		price *= 1 + 0.001*float64(1-2*(i%2)) // up 0.1%, down 0.1%, ...
		candles = append(candles, coinbase.Candle{T: int64(i * 60), C: price})
	}
	vol, ok := minuteVol(candles)
	if !ok || vol < 0.0009 || vol > 0.0011 {
		t.Fatalf("vol %.5f ok %v, want about 0.001", vol, ok)
	}
	if _, ok := minuteVol(candles[:10]); ok {
		t.Fatal("measured a volatility from 9 returns")
	}
}

func TestPriceRangeFarAndContinuousRange(t *testing.T) {
	s, err := newPriceRange(priceParams(true, 30))
	if err != nil {
		t.Fatal(err)
	}
	r := s.(*priceRange)
	r.coins["BTC"] = &coinState{price: 60000, minuteVol: 0.0005, priceAt: testNow, volAt: testNow}
	near := Market{Event: "KXBTCD-26NOV2123", Strike: STRIKE_GREATER, Floor: 60050, CloseAt: testNow.Add(10 * time.Minute), YesAsk: 0.4, NoAsk: 0.62}
	if _, ok := r.Evaluate(near, testNow); ok {
		t.Fatal("far-only took a line $50 away with 10 minutes left")
	}
	away := near
	away.Floor, away.YesAsk, away.NoAsk = 61500, 0.03, 0.98
	op, ok := r.Evaluate(away, testNow)
	if !ok || op.Side != SIDE_NO || op.Prob > 0.001 {
		t.Fatalf("got %+v %v, want NO with almost no chance of YES", op, ok)
	}
	if lo, hi, ok := (Market{Strike: STRIKE_BETWEEN, Floor: 0.91, Cap: 0.9299}).ContinuousRange(); !ok || lo != 0.91 || hi != 0.9299 {
		t.Fatalf("continuous range %v %v: prices are not rounded like degrees", lo, hi)
	}
	late := away
	late.CloseAt = testNow.Add(2 * time.Hour)
	if _, ok := r.Evaluate(late, testNow); ok {
		t.Fatal("took a market past max_minutes")
	}
	away.Floor = 90000
	if op, ok := r.Evaluate(away, testNow); !ok || op.Prob != 0 || !op.HasProb {
		t.Errorf("probability that saturated at zero lost its model status: %+v", op)
	}
	if _, ok := r.Evaluate(away, testNow.Add(TICKER_EVERY)); ok {
		t.Error("formed an opinion after the exchange quote expired")
	}
	r.coins["BTC"].priceAt = testNow.Add(VOL_EVERY)
	if _, ok := r.Evaluate(away, testNow.Add(VOL_EVERY)); ok {
		t.Error("formed an opinion after volatility expired")
	}
}

func TestTrimPricePreservesIntegerZeros(t *testing.T) {
	if got := trimPrice(600000); got != "600000" {
		t.Fatalf("displayed price %q, want 600000", got)
	}
}

func TestNewDefaultsJoinOnceAndDeletionsStick(t *testing.T) {
	path := filepath.Join(t.TempDir(), RULEBOOK_FILE)
	old := DefaultRulebook()
	old.Strategies, old.DefaultsSeen = old.Strategies[:3], nil // a rulebook from before the crypto strategies
	if err := SaveRulebook(path, old); err != nil {
		t.Fatal(err)
	}
	rb, err := LoadRulebook(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rb.Strategies) != len(DefaultRulebook().Strategies) {
		t.Fatalf("%d strategies after offering the new built-ins, want %d", len(rb.Strategies), len(DefaultRulebook().Strategies))
	}
	rb.Strategies = rb.Strategies[:len(rb.Strategies)-1] // the owner deletes one
	if err := SaveRulebook(path, rb); err != nil {
		t.Fatal(err)
	}
	again, err := LoadRulebook(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Strategies) != len(rb.Strategies) {
		t.Fatal("a deleted built-in strategy came back")
	}
}
