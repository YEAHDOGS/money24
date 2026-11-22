package news

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	LIVE_ENV            = "M24D_LIVE"
	LIVE_SOURCES_ENV    = "M24D_SOURCES" // a sources file to check instead of the embedded one
	LIVE_ONLY_ENV       = "M24D_ONLY"    // a source id prefix: check only those
	LIVE_AGENT_ENV      = "USER_AGENT"   // as in production: SEC and BLS answer 403 until it carries a contact
	LIVE_DEFAULT_UA     = "Money24-News/0.1 (DOGS)"
	LIVE_TIMEOUT        = 30 * time.Second
	LIVE_GDELT_WAIT     = 3 * time.Minute     // GDELT sources take turns, 20 s apart
	LIVE_MAX_AGE        = 7 * 24 * time.Hour  // a news feed whose newest item is older than this counts as stale
	LIVE_MAX_AGE_SLOW   = 45 * 24 * time.Hour // monthly releases (macro, central banks, filings) may be quiet for weeks
	LIVE_REVALIDATE_ENV = "M24D_REVALIDATE"   // ask each feed a second time at once: does it answer 304, and what does that cost
	LIVE_MIN_ITEMS      = 1
	LIVE_NEEDS_UA       = "sec.gov bls.gov" // hosts that refuse the default User-Agent
)

// TestLiveSources loads sources.json and fetches every source once through the real fetcher and parser.
// Each must answer with items that have a title, a web link and a date, and a recent newest item.
// Skipped unless M24D_LIVE=1.
func TestLiveSources(t *testing.T) {
	if os.Getenv(LIVE_ENV) == "" {
		t.Skip("set M24D_LIVE=1 to fetch every source once")
	}
	sources, err := LoadSources(os.Getenv(LIVE_SOURCES_ENV))
	if err != nil {
		t.Fatal(err)
	}
	tagger, err := LoadTagger("")
	if err != nil {
		t.Fatal(err)
	}
	agent := os.Getenv(LIVE_AGENT_ENV)
	monitor := NewMonitor(sources)
	fetcher := NewFetcher(LIVE_TIMEOUT, firstNonEmpty(agent, LIVE_DEFAULT_UA), monitor)

	only := os.Getenv(LIVE_ONLY_ENV)
	for _, src := range sources {
		if !strings.HasPrefix(src.ID, only) {
			continue
		}
		t.Run(src.ID, func(t *testing.T) {
			t.Parallel() // capped by go test -parallel
			if agent == "" && needsContactAgent(src) {
				t.Skipf("%s refuses the default User-Agent: set USER_AGENT with a contact to check it", src.ID)
			}
			checkLiveSource(t, src, fetcher, monitor, tagger)
		})
	}
}

func needsContactAgent(src Source) bool {
	for _, host := range strings.Fields(LIVE_NEEDS_UA) {
		if strings.Contains(src.URL, host) {
			return true
		}
	}
	return false
}

func checkLiveSource(t *testing.T, src Source, fetcher *Fetcher, monitor *Monitor, tagger *Tagger) {
	timeout := LIVE_TIMEOUT
	if src.Type == TYPE_GDELT {
		timeout = LIVE_GDELT_WAIT
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	batch, err := POLLERS[src.Type](ctx, src, fetcher, tagger)
	if err != nil {
		t.Errorf("FAIL %s: %v", src.ID, err)
		return
	}
	monitor.mu.Lock()
	wire := monitor.sources[src.ID].LastIn
	monitor.mu.Unlock()
	if src.Type == TYPE_POLYMARKET {
		if len(batch.Markets) == 0 {
			t.Errorf("FAIL %s: no markets", src.ID)
		}
		return
	}
	if len(batch.Events) < LIVE_MIN_ITEMS {
		t.Errorf("FAIL %s: no items", src.ID)
		return
	}

	var newest, oldest *time.Time
	dated := 0
	for _, e := range batch.Events {
		if e.Title == "" || e.URL == "" {
			t.Errorf("FAIL %s: item without title or link", src.ID)
			return
		}
		if e.PublishedAt == nil {
			continue
		}
		dated++
		if newest == nil || e.PublishedAt.After(*newest) {
			newest = e.PublishedAt
		}
		if oldest == nil || e.PublishedAt.Before(*oldest) {
			oldest = e.PublishedAt
		}
	}
	if newest == nil {
		t.Errorf("FAIL %s: %d items, none dated", src.ID, len(batch.Events))
		return
	}
	limit := LIVE_MAX_AGE
	if src.Kind != "news" {
		limit = LIVE_MAX_AGE_SLOW
	}
	verdict := "ok"
	if age := time.Since(*newest); age > limit {
		verdict = "STALE"
		t.Errorf("STALE %s: newest item is %s old", src.ID, age.Round(time.Hour))
	}
	first := batch.Events[0]
	revalidated := ""
	if os.Getenv(LIVE_REVALIDATE_ENV) != "" {
		revalidated = revalidate(ctx, src, fetcher, monitor, tagger)
	}
	t.Logf("%s %s: %d items (%d dated), newest %s ago, oldest %s ago, %.1f KB on the wire, %s%s | %q %s",
		verdict, src.ID, len(batch.Events), dated, time.Since(*newest).Round(time.Minute), time.Since(*oldest).Round(time.Hour),
		float64(wire)/1024, time.Since(started).Round(time.Millisecond), revalidated, first.Title, first.URL)
}

// revalidate polls a source a second time at once and reports whether the feed answered 304 Not Modified
// and the bytes that second request cost, so the run can estimate what steady polling costs.
func revalidate(ctx context.Context, src Source, fetcher *Fetcher, monitor *Monitor, tagger *Tagger) string {
	batch, err := POLLERS[src.Type](ctx, src, fetcher, tagger)
	monitor.mu.Lock()
	again := monitor.sources[src.ID].LastIn
	monitor.mu.Unlock()
	if err != nil {
		return fmt.Sprintf(", again: %v", err)
	}
	return fmt.Sprintf(", again 304=%v %.1f KB", batch.Unchanged, float64(again)/1024)
}
