package news

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// The calendar is the second half of newsd: scheduled events rather than headlines. Each entry in
// calendars.json names a parser type; adding a calendar is one JSON line when a parser for its
// format exists (any iCalendar URL works with "ics"), or one function in CAL_PARSERS when not.

const (
	CALENDARS_FILE      = "calendars.json"
	EVENT_CALENDAR      = "calendar"
	CAL_TICK            = time.Minute
	CAL_DEFAULT_EVERY   = time.Hour
	CAL_MIN_EVERY       = 5 * time.Minute // no calendar is worth asking for more often
	CAL_MAX_DAYS        = 31
	CAL_REQUEST_GAP     = time.Second // pause between the per-day requests of one source
	CAL_WINDOW_BACK     = 7 * 24 * time.Hour
	CAL_WINDOW_FORWARD  = 28 * 24 * time.Hour
	CAL_MAX_ROWS        = 4000
	TOKEN_TODAY         = "{today}"
	TOKEN_DATE          = "{date}"
	CATEGORY_MACRO      = "macro"
	CATEGORY_EARNINGS   = "earnings"
	CATEGORY_AUCTION    = "auction"
	CATEGORY_HOLIDAY    = "holiday"
	CATEGORY_OTHER      = "other"
	IMPACT_LOW          = "low"
	IMPACT_MEDIUM       = "medium"
	IMPACT_HIGH         = "high"
	UID_SEPARATOR       = "|"
	HOURS_PER_DAY       = 24
	CAL_TITLE_MAX_CHARS = 200
)

// CalSource is one entry in calendars.json. A URL may hold {today} or {date}; with Days set, the
// source is fetched once per day ahead, with {date} filled in for each.
type CalSource struct {
	ID           string        `json:"id"`
	Label        string        `json:"label"`
	Type         string        `json:"type"`
	Category     string        `json:"category"`
	URL          string        `json:"url"`
	MinEveryRaw  string        `json:"min_every,omitempty"`
	Days         int           `json:"days,omitempty"`
	MinMarketCap float64       `json:"min_market_cap,omitempty"`
	MinEvery     time.Duration `json:"-"`
}

// CalEvent is one scheduled event. For all-day events StartsAt is 00:00 UTC of that date.
type CalEvent struct {
	UID      string    `json:"uid"`
	Source   string    `json:"source"`
	Category string    `json:"category"`
	StartsAt time.Time `json:"starts_at"`
	AllDay   bool      `json:"all_day"`
	Title    string    `json:"title"`
	Region   string    `json:"region,omitempty"`
	Symbol   string    `json:"symbol,omitempty"`
	Impact   string    `json:"impact,omitempty"`
	Forecast string    `json:"forecast,omitempty"`
	Previous string    `json:"previous,omitempty"`
	Actual   string    `json:"actual,omitempty"`
	Detail   string    `json:"detail,omitempty"`
	URL      string    `json:"url,omitempty"`
}

// CalParser turns one response body into events. day is the date the URL was built for.
type CalParser func(body []byte, src CalSource, day time.Time) ([]CalEvent, error)

// CAL_PARSERS maps a calendars.json "type" to its parser. Add a format here.
var CAL_PARSERS = map[string]CalParser{
	"ff_week":           parseFFWeek,
	"treasury_auctions": parseTreasuryAuctions,
	"nasdaq_earnings":   parseNasdaqEarnings,
	"ics":               parseICS,
}

// LoadCalendars reads and validates calendars.json (embedded unless path is set).
func LoadCalendars(path string) ([]CalSource, error) {
	raw, err := readConfig(path, CALENDARS_FILE)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Calendars []CalSource `json:"calendars"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i := range doc.Calendars {
		src := &doc.Calendars[i]
		if src.ID == "" || seen[src.ID] {
			return nil, fmt.Errorf("calendar %d: missing or duplicate id %q", i, src.ID)
		}
		seen[src.ID] = true
		if _, ok := CAL_PARSERS[src.Type]; !ok {
			return nil, fmt.Errorf("%s: unknown calendar type %q", src.ID, src.Type)
		}
		if src.Days < 0 || src.Days > CAL_MAX_DAYS {
			return nil, fmt.Errorf("%s: days must be 0 to %d", src.ID, CAL_MAX_DAYS)
		}
		src.Label = firstNonEmpty(src.Label, src.ID)
		src.Category = firstNonEmpty(src.Category, CATEGORY_OTHER)
		src.MinEvery = CAL_DEFAULT_EVERY
		if src.MinEveryRaw == "" {
			continue
		}
		every, err := time.ParseDuration(src.MinEveryRaw)
		if err != nil {
			return nil, fmt.Errorf("%s: bad min_every %q", src.ID, src.MinEveryRaw)
		}
		src.MinEvery = max(every, CAL_MIN_EVERY)
	}
	return doc.Calendars, nil
}

// calendarLoop polls each calendar on its own slow clock. The news poll slider does not touch it.
func (s *Service) calendarLoop(ctx context.Context) {
	last := map[string]time.Time{}
	ticker := time.NewTicker(CAL_TICK)
	defer ticker.Stop()
	for {
		for _, src := range s.calendars {
			if time.Since(last[src.ID]) < src.MinEvery {
				continue
			}
			last[src.ID] = time.Now()
			s.pollCalendar(ctx, src)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// pollCalendar fetches one calendar (one request per day ahead when Days is set), stores what
// changed and tells every open window to reload.
func (s *Service) pollCalendar(ctx context.Context, src CalSource) {
	today := time.Now().UTC().Truncate(HOURS_PER_DAY * time.Hour)
	changed := 0
	for i := range max(src.Days, 1) {
		if i > 0 && !sleepCtx(ctx, CAL_REQUEST_GAP) {
			return
		}
		day := today.AddDate(0, 0, i)
		url := strings.NewReplacer(TOKEN_TODAY, today.Format(DAY_LAYOUT), TOKEN_DATE, day.Format(DAY_LAYOUT)).Replace(src.URL)
		body, err := s.fetcher.get(ctx, src.ID, url)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("[%s] %v", src.ID, err)
			}
			return
		}
		if body == nil {
			continue // 304: nothing changed
		}
		events, err := CAL_PARSERS[src.Type](body, src, day)
		if err != nil {
			log.Printf("[%s] parse: %v", src.ID, err)
			return
		}
		n, err := s.store.syncCalendar(ctx, src.ID, events)
		if err != nil {
			log.Printf("[%s] store: %v", src.ID, err)
			return
		}
		changed += n
	}
	s.monitor.recordAdded(src.ID, changed)
	if changed == 0 {
		return
	}
	log.Printf("[%s] calendar: %d changed", src.ID, changed)
	data, _ := json.Marshal(map[string]int{"changed": changed})
	s.hub.broadcast(Message{Event: EVENT_CALENDAR, Data: data})
}

// sleepCtx waits for d, or returns false when ctx ends first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// calEvent fills in what every parser shares: a namespaced uid, the source and a clean title.
func calEvent(src CalSource, key string, at time.Time, allDay bool, title string) CalEvent {
	return CalEvent{
		UID:      src.ID + UID_SEPARATOR + key,
		Source:   src.ID,
		Category: src.Category,
		StartsAt: at.UTC().Truncate(time.Millisecond),
		AllDay:   allDay,
		Title:    clean(title, CAL_TITLE_MAX_CHARS),
	}
}
