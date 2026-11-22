package meme

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

const (
	LIVE_ENV     = "M24D_LIVE"
	LIVE_ENABLED = "1"
	LIVE_WINDOW  = time.Minute
)

func TestLiveRecorder(t *testing.T) {
	if os.Getenv(LIVE_ENV) != LIVE_ENABLED {
		t.Skip("set M24D_LIVE=1 for the public feed smoke test")
	}
	recorder := Recorder{Dir: t.TempDir()}
	ctx, cancel := context.WithTimeout(context.Background(), LIVE_WINDOW)
	defer cancel()
	recorder.Run(ctx)
	t.Logf("one-minute status: %+v", recorder.Status())

	files, err := RecordFiles(recorder.Dir, recorder.Status().Day)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	samples := make(map[string]string)
	priced, safetyReports := 0, 0
	for _, name := range files {
		f, err := OpenRecords(recorder.Dir, name)
		if err != nil {
			t.Fatal(err)
		}
		for {
			line, _, err := f.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				f.Close()
				t.Fatal(err)
			}
			var row Record
			if err := json.Unmarshal(line, &row); err != nil {
				f.Close()
				t.Fatal(err)
			}
			counts[row.Kind]++
			switch row.Kind {
			case KIND_SNAPSHOT:
				if row.Market != nil && row.Market.PriceUSD != nil {
					priced++
					if samples[KIND_SNAPSHOT] == "" {
						samples[KIND_SNAPSHOT] = string(line)
					}
				}
			case KIND_SAFETY:
				if row.Safety != nil {
					safetyReports++
				}
			case KIND_LAUNCH, KIND_GRADUATION:
				if samples[row.Kind] == "" {
					samples[row.Kind] = string(line)
				}
			}
		}
		f.Close()
	}
	t.Logf("disk counts: %v, priced snapshots: %d, safety reports: %d", counts, priced, safetyReports)
	for _, kind := range []string{KIND_LAUNCH, KIND_GRADUATION, KIND_SNAPSHOT} {
		if sample := samples[kind]; sample != "" {
			t.Logf("%s sample: %s", kind, sample)
		}
	}
	if counts[KIND_LAUNCH] == 0 || priced == 0 || safetyReports == 0 {
		t.Fatal("expected a launch, a priced snapshot and a successful safety report")
	}
}
