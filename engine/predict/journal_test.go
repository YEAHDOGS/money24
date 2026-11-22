package predict

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenBookPreservesNextBuyAfterUnterminatedJournalLine(t *testing.T) {
	for _, tail := range []string{
		`{"id":1,"kind":"watch","ticker":"OLDER","price":0.90}`,
		`{"id":1,"kind":"buy","ticker":"INCOMPLETE"`,
	} {
		t.Run(tail, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "book.jsonl")
			if err := os.WriteFile(path, []byte(tail), FILE_PERMS); err != nil {
				t.Fatal(err)
			}
			b, err := OpenBook(path, 100)
			if err != nil {
				t.Fatal(err)
			}
			_, appendErr := b.Append(Line{At: testNow, Kind: KIND_BUY, Ticker: "T", Side: SIDE_YES, Qty: 1, CostUSD: 0.9, FeeUSD: 0.01})
			b.Close()
			if appendErr != nil {
				t.Fatal(appendErr)
			}
			again, err := OpenBook(path, 100)
			if err != nil {
				t.Fatal(err)
			}
			defer again.Close()
			if a := again.Account(""); a.CashUSD != 99.09 || again.Holding("T") == nil {
				t.Fatalf("buy missing after reopen: %+v", a)
			}
		})
	}
}

func TestOpenBookRejectsJournalWhenReplayStopsEarly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.jsonl")
	doc := strings.Repeat(" ", LINE_MAX) + "\n" + `{"id":2,"kind":"buy","ticker":"UNREAD","cost_usd":50}` + "\n"
	if err := os.WriteFile(path, []byte(doc), FILE_PERMS); err != nil {
		t.Fatal(err)
	}
	b, err := OpenBook(path, 100)
	if err == nil {
		b.Close()
		t.Fatal("opened a journal with buys the scanner failed to replay")
	}
}

func TestRecordUsesOneContractFeeAndCountsProfitableOutcomes(t *testing.T) {
	b, err := OpenBook(filepath.Join(t.TempDir(), "book.jsonl"), 100)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for _, l := range []Line{
		{At: testNow, Kind: KIND_WATCH, Ticker: "T", Side: SIDE_YES, Price: 0.9},
		{At: testNow, Kind: KIND_SETTLE, Ticker: "T", Result: "scalar", Value: 0.905},
		{At: testNow, Kind: KIND_WATCH, Ticker: "ZERO", Side: SIDE_YES},
		{At: testNow, Kind: KIND_SETTLE, Ticker: "ZERO", Result: RESULT_NO},
	} {
		if _, err := b.Append(l); err != nil {
			t.Fatal(err)
		}
	}
	record := b.Record(FAVORITES_ID, []float64{0, 0.5, 1})
	band := record.Bands[1]
	if band.AvgCost != 0.91 || band.EdgePct != -0.55 || band.Won != 0 || record.Won != 0 {
		t.Errorf("band %+v, want $0.91 cost, -0.55%% edge and no win", band)
	}
	if _, err := json.Marshal(record); err != nil {
		t.Errorf("legacy zero-cost watch made its entire record fail JSON encoding: %v", err)
	}
}
