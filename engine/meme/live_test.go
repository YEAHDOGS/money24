package meme

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

	files, err := filepath.Glob(filepath.Join(recorder.Dir, "*", RECORD_PATTERN))
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	samples := make(map[string]string)
	priced, safetyReports := 0, 0
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, RESTORE_BUFFER), MAX_LOOKUP_BYTES)
		for scanner.Scan() {
			var row Record
			if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
				f.Close()
				t.Fatal(err)
			}
			counts[row.Kind]++
			switch row.Kind {
			case KIND_SNAPSHOT:
				if row.Market != nil && row.Market.PriceUSD != nil {
					priced++
					if samples[KIND_SNAPSHOT] == "" {
						samples[KIND_SNAPSHOT] = scanner.Text()
					}
				}
			case KIND_SAFETY:
				if row.Safety != nil {
					safetyReports++
				}
			case KIND_LAUNCH, KIND_GRADUATION:
				if samples[row.Kind] == "" {
					samples[row.Kind] = scanner.Text()
				}
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
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
