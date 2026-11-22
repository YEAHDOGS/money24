package tape

import (
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/market"
)

func TestWriteAppendsAndReadsBack(t *testing.T) {
	r := &Recorder{Dir: t.TempDir()}
	at := time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)
	for minute := range 3 {
		snap := Snapshot{At: at.Add(time.Duration(minute) * time.Minute), Spot: 690 + float64(minute), Expiry: "2026-11-23",
			Calls: []market.OptionRow{{Contract: "SPY261123C00690000", Strike: 690, Bid: 1.2, Ask: 1.25, Volume: 1000 * (minute + 1)}}}
		if err := r.write("2026-11-23", "^SPX", []Snapshot{snap}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Read(r.Dir, "2026-11-23", "^SPX")
	if err != nil || len(got) != 3 {
		t.Fatalf("read %d snapshots, err %v, want 3", len(got), err)
	}
	if got[2].Spot != 692 || got[2].Calls[0].Volume != 3000 {
		t.Errorf("last snapshot %+v", got[2])
	}
}

func TestSessionFollowsTheCalendar(t *testing.T) {
	ny := func(day, clock string) time.Time {
		v, _ := time.ParseInLocation("2006-01-02 15:04", day+" "+clock, time.FixedZone("EST", -5*3600))
		return v
	}
	cases := []struct {
		at   time.Time
		want bool
	}{
		{ny("2026-11-23", "09:29"), true},  // a minute early for the first snapshot
		{ny("2026-11-23", "09:27"), false}, // before that
		{ny("2026-11-23", "16:10"), true},  // delayed quotes still coming in
		{ny("2026-11-23", "16:20"), false},
		{ny("2026-11-26", "12:00"), false}, // Thanksgiving
		{ny("2026-11-27", "13:10"), true},  // half day: 1 PM close, plus the delay
		{ny("2026-11-27", "13:20"), false},
		{ny("2026-11-21", "12:00"), false}, // Saturday
	}
	for _, c := range cases {
		if got := session(c.at); got != c.want {
			t.Errorf("%s: recording %v, want %v", c.at, got, c.want)
		}
	}
}
