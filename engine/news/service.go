package news

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

const (
	USAGE_FLUSH_EVERY = 10 * time.Second
	STATS_EVERY       = 3 * time.Second
	EVENT_STATS       = "stats"
	EVENT_SETTINGS    = "settings"
	MONTH_LAYOUT      = "2006-01"
	FIRST_DAY_SUFFIX  = "-01"
)

// Config is everything a Service needs from the command line.
type Config struct {
	DataDir       string
	UserAgent     string
	SourcesFile   string
	WatchlistFile string
	HTTPTimeout   time.Duration
}

// Service ties the pollers, the store, the meter and the live hub together.
type Service struct {
	store    *Store
	hub      *Hub
	monitor  *Monitor
	fetcher  *Fetcher
	tagger   *Tagger
	sources  []Source
	schedule *Schedule
	clock    clock
}

// Settings is what the UI can read and change.
type Settings struct {
	PollEveryMs int64 `json:"poll_every_ms"`
	MinMs       int64 `json:"min_ms"`
	MaxMs       int64 `json:"max_ms"`
}

// NewService loads config, opens the database and restores the saved poll interval.
func NewService(ctx context.Context, cfg Config) (*Service, error) {
	sources, err := LoadSources(cfg.SourcesFile)
	if err != nil {
		return nil, err
	}
	tagger, err := LoadTagger(cfg.WatchlistFile)
	if err != nil {
		return nil, err
	}
	store, err := OpenStore(ctx, cfg.DataDir)
	if err != nil {
		return nil, err
	}
	monitor := NewMonitor(sources)
	return &Service{
		store:    store,
		hub:      NewHub(),
		monitor:  monitor,
		fetcher:  NewFetcher(cfg.HTTPTimeout, cfg.UserAgent, monitor),
		tagger:   tagger,
		sources:  sources,
		schedule: NewSchedule(store.pollEvery(ctx, DEFAULT_POLL)),
	}, nil
}

// Summary describes the loaded config for the startup log line.
func (s *Service) Summary() (sources, tags int, every time.Duration) {
	every, _ = s.schedule.Every()
	return len(s.sources), s.tagger.Size(), every
}

// Run polls every source on the shared schedule until ctx ends, then flushes the meter.
func (s *Service) Run(ctx context.Context) {
	go s.cycleLoop(ctx)
	go s.statsLoop(ctx)
	s.flushLoop(ctx)
	s.store.Close()
}

// pollOnce polls one source, stores what is new and streams it to every open window.
func (s *Service) pollOnce(ctx context.Context, src Source) pollOutcome {
	outcome := pollOutcome{id: src.ID}
	batch, err := POLLERS[src.Type](ctx, src, s.fetcher, s.tagger)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("[%s] %v", src.ID, err)
		}
		outcome.err = err
		return outcome
	}
	outcome.unchanged = batch.Unchanged
	if err := s.store.upsertMarkets(ctx, batch.Markets); err != nil {
		log.Printf("[%s] markets: %v", src.ID, err)
	}
	added, err := s.store.insertEvents(ctx, batch.Events)
	if err != nil {
		log.Printf("[%s] store: %v", src.ID, err)
		outcome.err = err
		return outcome
	}
	outcome.added = len(added)
	s.monitor.recordAdded(src.ID, len(added))
	for _, e := range added {
		data, _ := json.Marshal(e)
		s.hub.broadcast(Message{ID: e.ID, Data: data})
	}
	if len(added) > 0 {
		log.Printf("[%s] +%d", src.ID, len(added))
	}
	return outcome
}

// flushLoop writes the meter into the daily usage ledger, and once more on shutdown.
func (s *Service) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(USAGE_FLUSH_EVERY)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.flushUsage(context.Background())
			return
		case <-ticker.C:
			s.flushUsage(ctx)
		}
	}
}

func (s *Service) flushUsage(ctx context.Context) {
	day := time.Now().UTC().Format(DAY_LAYOUT)
	if err := s.store.addUsage(ctx, day, s.monitor.drain()); err != nil {
		log.Printf("[usage] %v", err)
	}
}

// statsLoop pushes meter changes to open streams. Only changed sources are sent, to keep our own egress small.
func (s *Service) statsLoop(ctx context.Context) {
	ticker := time.NewTicker(STATS_EVERY)
	defer ticker.Stop()
	var since uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if s.hub.count() == 0 {
			continue
		}
		snap := s.Snapshot(ctx, since)
		since = snap.Version
		data, _ := json.Marshal(snap)
		s.hub.broadcast(Message{Event: EVENT_STATS, Data: data})
	}
}

// Snapshot is the network monitor's view: live meters plus this month's ledger.
func (s *Service) Snapshot(ctx context.Context, since uint64) NetSnapshot {
	snap := s.monitor.snapshot(since)
	every, _ := s.schedule.Every()
	snap.EveryMs = every.Milliseconds()
	snap.Clients = s.hub.count()
	snap.MonthStart = time.Now().UTC().Format(MONTH_LAYOUT) + FIRST_DAY_SUFFIX
	month, err := s.store.usageSince(ctx, snap.MonthStart)
	if err != nil {
		log.Printf("[usage] %v", err)
	}
	snap.Month = month
	return snap
}

// Settings returns the current poll interval and its bounds.
func (s *Service) Settings() Settings {
	every, _ := s.schedule.Every()
	return Settings{PollEveryMs: every.Milliseconds(), MinMs: MIN_POLL.Milliseconds(), MaxMs: MAX_POLL.Milliseconds()}
}

// SetPollEvery changes the global interval, saves it and tells every open window.
func (s *Service) SetPollEvery(ctx context.Context, every time.Duration) (Settings, error) {
	if err := s.schedule.Set(every); err != nil {
		return Settings{}, err
	}
	if err := s.store.savePollEvery(ctx, every); err != nil {
		return Settings{}, err
	}
	settings := s.Settings()
	data, _ := json.Marshal(settings)
	s.hub.broadcast(Message{Event: EVENT_SETTINGS, Data: data})
	// Re-plan the ring now; the clock confirms with its own plan once the current round reports.
	plan, _ := s.clock.snapshot()
	s.announcePlan(Plan{LastAt: plan.LastAt, NextAt: maxTime(plan.LastAt.Add(every), time.Now().UTC()), EveryMs: every.Milliseconds()})
	log.Printf("poll interval set to %s", every)
	return settings, nil
}
