package calendar

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	TEST_STAMP = "2006-01-02 15:04"
	NOON       = " 12:00"
)

// at reads "2026-11-21 09:30" as New York time.
func at(t *testing.T, stamp string) time.Time {
	t.Helper()
	out, err := time.ParseInLocation(TEST_STAMP, stamp, NEW_YORK)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func tagIDs(d Day) string {
	ids := make([]string, 0, len(d.Tags))
	for _, tag := range d.Tags {
		ids = append(ids, tag.ID)
	}
	return "," + strings.Join(ids, ",") + ","
}

func hasTag(d Day, id string) bool { return strings.Contains(tagIDs(d), ","+id+",") }

// The exchange's published calendar for 2026 and 2027, plus the 2028 dates that settle the rules:
// New Year's Day on a Saturday is not kept, and July 3 is a half day only before a weekday July 4.
func TestSessionsMatchTheExchangeCalendar(t *testing.T) {
	cases := []struct{ date, session, holiday string }{
		{"2026-01-01", SESSION_CLOSED, "New Year's Day"},
		{"2026-01-19", SESSION_CLOSED, "Martin Luther King Jr. Day"},
		{"2026-02-16", SESSION_CLOSED, "Washington's Birthday"},
		{"2026-04-03", SESSION_CLOSED, "Good Friday"},
		{"2026-05-25", SESSION_CLOSED, "Memorial Day"},
		{"2026-06-19", SESSION_CLOSED, "Juneteenth"},
		{"2026-07-02", SESSION_OPEN, ""}, // July 4 is a Saturday: the Thursday is a full day
		{"2026-07-03", SESSION_CLOSED, "Independence Day"},
		{"2026-09-07", SESSION_CLOSED, "Labor Day"},
		{"2026-11-25", SESSION_OPEN, ""},
		{"2026-11-26", SESSION_CLOSED, "Thanksgiving"},
		{"2026-11-27", SESSION_HALF, ""},
		{"2026-12-24", SESSION_HALF, ""},
		{"2026-12-25", SESSION_CLOSED, "Christmas"},
		{"2026-12-31", SESSION_OPEN, ""},
		{"2027-01-01", SESSION_CLOSED, "New Year's Day"},
		{"2027-01-18", SESSION_CLOSED, "Martin Luther King Jr. Day"},
		{"2027-02-15", SESSION_CLOSED, "Washington's Birthday"},
		{"2027-03-26", SESSION_CLOSED, "Good Friday"},
		{"2027-05-31", SESSION_CLOSED, "Memorial Day"},
		{"2027-06-18", SESSION_CLOSED, "Juneteenth"}, // June 19 is a Saturday
		{"2027-07-02", SESSION_OPEN, ""},
		{"2027-07-05", SESSION_CLOSED, "Independence Day"}, // July 4 is a Sunday
		{"2027-09-06", SESSION_CLOSED, "Labor Day"},
		{"2027-11-25", SESSION_CLOSED, "Thanksgiving"},
		{"2027-11-26", SESSION_HALF, ""},
		{"2027-12-23", SESSION_OPEN, ""},
		{"2027-12-24", SESSION_CLOSED, "Christmas"}, // December 25 is a Saturday
		{"2027-12-31", SESSION_OPEN, ""},            // New Year's Day 2028 is a Saturday: not kept
		{"2028-07-03", SESSION_HALF, ""},
		{"2028-07-04", SESSION_CLOSED, "Independence Day"},
		{"2028-11-24", SESSION_HALF, ""},
		{"2028-12-22", SESSION_OPEN, ""}, // Christmas Eve is a Sunday: no half day
		{"2028-12-25", SESSION_CLOSED, "Christmas"},
		{"2021-06-18", SESSION_OPEN, ""}, // before the exchanges kept Juneteenth
	}
	for _, c := range cases {
		day := On(at(t, c.date+NOON))
		if day.Session != c.session || day.Holiday != c.holiday {
			t.Errorf("%s: session %q holiday %q, want %q %q", c.date, day.Session, day.Holiday, c.session, c.holiday)
		}
		if (day.Session == SESSION_CLOSED) != day.Opens.IsZero() {
			t.Errorf("%s: opens %v on a %s day", c.date, day.Opens, day.Session)
		}
	}
}

// A year has ten holidays, or nine when New Year's Day falls on a Saturday.
func TestHolidayCountPerYear(t *testing.T) {
	for year, want := range map[int]int{2026: 10, 2027: 10, 2028: 9} {
		got := 0
		for d := time.Date(year, time.January, 1, 0, 0, 0, 0, NEW_YORK); d.Year() == year; d = d.AddDate(0, 0, 1) {
			if holidayOn(d) != "" {
				got++
			}
		}
		if got != want {
			t.Errorf("%d: %d holidays, want %d", year, got, want)
		}
	}
}

func TestStocksOpenAtTheEdges(t *testing.T) {
	cases := []struct {
		stamp string
		open  bool
	}{
		{"2026-11-23 09:29", false},
		{"2026-11-23 09:30", true},
		{"2026-11-23 15:59", true},
		{"2026-11-23 16:00", false},
		{"2026-11-27 12:59", true},
		{"2026-11-27 13:00", false}, // half day
		{"2026-11-26 12:00", false}, // Thanksgiving
		{"2026-11-21 12:00", false}, // Saturday
	}
	for _, c := range cases {
		if got := StocksOpen(at(t, c.stamp)); got != c.open {
			t.Errorf("%s: open %v, want %v", c.stamp, got, c.open)
		}
	}
	// The same instant given in another zone is still read on the New York clock.
	if !StocksOpen(at(t, "2026-11-23 09:30").UTC()) {
		t.Error("09:30 New York given as UTC should be open")
	}
}

func TestNextSettlementSkipsWeekendsAndHolidays(t *testing.T) {
	cases := []struct{ sold, settles string }{
		{"2026-11-20 14:00", "2026-11-23 09:30"}, // Friday sale is Monday's money
		{"2026-11-25 10:00", "2026-11-27 09:30"}, // over Thanksgiving, onto the half day
		{"2027-01-15 15:30", "2027-01-19 09:30"}, // Friday before a Monday holiday
		{"2026-11-23 10:00", "2026-11-24 09:30"},
		{"2026-11-21 12:00", "2026-11-23 09:30"}, // a weekend sale of something that settles T+1
	}
	for _, c := range cases {
		if got := NextSettlement(at(t, c.sold)); !got.Equal(at(t, c.settles)) {
			t.Errorf("sold %s: settles %s, want %s", c.sold, got.In(NEW_YORK).Format(TEST_STAMP), c.settles)
		}
	}
}

// Third Fridays in 2026. June's is Juneteenth, so that expiry moves to the Thursday.
func TestExpiries2026(t *testing.T) {
	monthly := map[time.Month]int{time.January: 16, time.February: 20, time.March: 20, time.April: 17, time.May: 15, time.June: 18,
		time.July: 17, time.August: 21, time.September: 18, time.October: 16, time.November: 20, time.December: 18}
	for d := time.Date(2026, time.January, 1, 0, 0, 0, 0, NEW_YORK); d.Year() == 2026; d = d.AddDate(0, 0, 1) {
		day := On(d)
		expiry := monthly[d.Month()] == d.Day()
		quarterly := d.Month()%MONTHS_PER_QUARTER == 0
		if hasTag(day, TAG_TRIPLE_WITCHING) != (expiry && quarterly) || hasTag(day, TAG_MONTHLY_EXPIRY) != (expiry && !quarterly) {
			t.Errorf("%s: tags %s, expiry %v quarterly %v", day.Date, tagIDs(day), expiry, quarterly)
		}
	}
	for date, want := range map[string]bool{"2026-12-16": true, "2026-05-19": true, "2026-05-20": false, "2026-11-18": true} {
		if got := hasTag(On(at(t, date+NOON)), TAG_VIX_EXPIRY); got != want {
			t.Errorf("%s: VIX expiration %v, want %v", date, got, want)
		}
	}
}

func TestTodayIsAWeekend(t *testing.T) {
	day := On(at(t, "2026-11-21 16:30"))
	if day.Weekday != "Saturday" || day.Session != SESSION_CLOSED || day.Label != "Weekend: crypto only" || day.Tags[0].ID != TAG_WEEKEND {
		t.Fatalf("got %s %s %q %s", day.Weekday, day.Session, day.Label, tagIDs(day))
	}
	if !day.NextOpen.Equal(at(t, "2026-11-23 09:30")) {
		t.Errorf("next open %v", day.NextOpen)
	}
	states := map[string]string{}
	for _, m := range day.Markets {
		states[m.Market] = m.State
		if m.Note == "" || strings.Contains(m.Note, "{") {
			t.Errorf("%s: note %q", m.Market, m.Note)
		}
	}
	want := map[string]string{MARKET_STOCKS: STATE_CLOSED, MARKET_OPTIONS: STATE_CLOSED, MARKET_FUTURES: STATE_CLOSED,
		MARKET_FOREX: STATE_CLOSED, MARKET_CRYPTO: STATE_THIN, MARKET_PREDICTION: STATE_ALWAYS}
	for market, state := range want {
		if states[market] != state {
			t.Errorf("%s: %s, want %s", market, states[market], state)
		}
	}
	for _, id := range []string{TAG_THANKSGIVING_WEEK, TAG_TAX_LOSS_SELLING, TAG_POST_MIDTERM, TAG_CYCLE_MIDTERM} {
		if !hasTag(day, id) {
			t.Errorf("missing %s in %s", id, tagIDs(day))
		}
	}
	if len(day.Playbook) == 0 || len(day.Playbook) > PLAYBOOK_MAX {
		t.Errorf("playbook %v", day.Playbook)
	}
}

func TestStructuralTags(t *testing.T) {
	cases := []struct {
		date  string
		label string
		has   []string
		lacks []string
	}{
		{"2026-11-20", "Monthly options expiry", []string{TAG_MONTHLY_EXPIRY, TAG_FRIDAY}, []string{TAG_PRE_HOLIDAY, TAG_FIRST_DAY_BACK}},
		{"2026-11-23", "First session back", []string{TAG_FIRST_DAY_BACK}, []string{TAG_FRIDAY, TAG_WEEKEND}},
		{"2026-11-24", "Normal session", nil, []string{TAG_FIRST_DAY_BACK, TAG_PRE_HOLIDAY}},
		{"2026-11-25", "Last session before Thanksgiving", []string{TAG_PRE_HOLIDAY}, []string{TAG_HALF_DAY}},
		{"2026-11-26", "Thanksgiving: stocks closed", []string{TAG_HOLIDAY}, []string{TAG_WEEKEND, TAG_FIRST_DAY_BACK}},
		{"2026-11-27", "Half day: closes 1 PM", []string{TAG_HALF_DAY, TAG_FIRST_DAY_BACK, TAG_FRIDAY}, []string{TAG_MONTH_END}},
		{"2026-11-30", "Month end", []string{TAG_MONTH_END, TAG_FIRST_DAY_BACK}, []string{TAG_QUARTER_END}},
		{"2026-12-01", "First session of the month", []string{TAG_MONTH_START}, []string{TAG_QUARTER_START}},
		{"2026-12-18", "Triple witching", []string{TAG_TRIPLE_WITCHING, TAG_FRIDAY}, []string{TAG_MONTHLY_EXPIRY}},
		{"2026-12-24", "Half day: closes 1 PM", []string{TAG_HALF_DAY, TAG_PRE_HOLIDAY, TAG_SANTA_RALLY}, nil},
		{"2026-12-31", "Last session before New Year's Day", []string{TAG_PRE_HOLIDAY, TAG_YEAR_END}, []string{TAG_QUARTER_END, TAG_MONTH_END}},
		{"2027-01-04", "First session of the quarter", []string{TAG_QUARTER_START, TAG_FIRST_DAY_BACK, TAG_SANTA_RALLY, TAG_JANUARY_EFFECT, TAG_CYCLE_PRE, TAG_POST_MIDTERM}, []string{TAG_MONTH_START}},
		{"2027-01-15", "Monthly options expiry", []string{TAG_PRE_HOLIDAY, TAG_FRIDAY, TAG_EARNINGS_SEASON}, []string{TAG_SANTA_RALLY}},
		{"2026-09-30", "Quarter end", []string{TAG_QUARTER_END, TAG_SEPTEMBER, TAG_SELL_IN_MAY}, []string{TAG_MONTH_END, TAG_YEAR_END}},
		{"2026-06-18", "Triple witching", []string{TAG_TRIPLE_WITCHING, TAG_PRE_HOLIDAY}, []string{TAG_FRIDAY}},
		{"2026-12-23", "Normal session", []string{TAG_TAX_LOSS_SELLING}, []string{TAG_SANTA_RALLY}},
		{"2027-01-06", "Normal session", nil, []string{TAG_SANTA_RALLY}},
		{"2027-07-15", "Normal session", []string{TAG_EARNINGS_SEASON, TAG_SELL_IN_MAY}, []string{TAG_POST_MIDTERM, TAG_SUMMER_DOLDRUMS}},
	}
	for _, c := range cases {
		day := On(at(t, c.date+NOON))
		if day.Label != c.label {
			t.Errorf("%s: label %q, want %q", c.date, day.Label, c.label)
		}
		for _, id := range c.has {
			if !hasTag(day, id) {
				t.Errorf("%s: missing %s in %s", c.date, id, tagIDs(day))
			}
		}
		for _, id := range c.lacks {
			if hasTag(day, id) {
				t.Errorf("%s: should not have %s", c.date, id)
			}
		}
	}
}

// Every day of two years: nothing left blank, no placeholder left unfilled, the playbook within
// its limit, a weekday's playbook ending on the events line, and a next open that is in the future.
func TestEveryDayIsComplete(t *testing.T) {
	events := WORDING.Playbook[PLAYBOOK_EVENTS][0]
	for d := time.Date(2026, time.January, 1, 0, 0, 0, 0, NEW_YORK); d.Year() < 2028; d = d.AddDate(0, 0, 1) {
		day := On(d)
		raw, err := json.Marshal(day)
		if err != nil {
			t.Fatal(err)
		}
		weekday := d.Weekday() != time.Saturday && d.Weekday() != time.Sunday
		switch {
		case strings.Contains(string(raw), "{holiday") || strings.Contains(string(raw), "{next"):
			t.Errorf("%s: unfilled placeholder in %s", day.Date, raw)
		case day.Label == "" || len(day.Markets) != 6 || len(day.Tags) == 0:
			t.Errorf("%s: incomplete: %s", day.Date, raw)
		case len(day.Playbook) == 0 || len(day.Playbook) > PLAYBOOK_MAX:
			t.Errorf("%s: playbook of %d lines", day.Date, len(day.Playbook))
		case weekday != (day.Playbook[len(day.Playbook)-1] == events):
			t.Errorf("%s: events line misplaced: %v", day.Date, day.Playbook)
		case !day.NextOpen.After(d) || !StocksOpen(day.NextOpen):
			t.Errorf("%s: next open %v", day.Date, day.NextOpen)
		}
		for _, tag := range day.Tags {
			if tag.Name == "" || tag.Note == "" {
				t.Errorf("%s: tag %s has no wording", day.Date, tag.ID)
			}
		}
	}
}

// Every tag has its wording, and every structural one a label and playbook lines.
func TestWordingCoversEveryTag(t *testing.T) {
	for _, id := range TAG_ORDER {
		if text := WORDING.Tags[id]; text.Name == "" || text.Note == "" {
			t.Errorf("strings.json: tag %s needs a name and a note", id)
		}
	}
	for _, id := range TAG_ORDER {
		if id == TAG_SANTA_RALLY {
			break // seasonal tags follow: context only, no label, no playbook
		}
		if WORDING.Tags[id].Label == "" || len(WORDING.Playbook[id]) == 0 {
			t.Errorf("strings.json: structural tag %s needs a label and playbook lines", id)
		}
	}
	for _, key := range []string{PLAYBOOK_NORMAL, PLAYBOOK_EVENTS} {
		if len(WORDING.Playbook[key]) == 0 {
			t.Errorf("strings.json: playbook %s is empty", key)
		}
	}
}

// 9:30 in New York is 13:30 UTC in summer and 14:30 in winter; the open follows New York.
func TestOpenFollowsNewYorkAcrossClockChanges(t *testing.T) {
	for date, utcHour := range map[string]int{"2026-03-06": 14, "2026-03-09": 13, "2026-10-30": 13, "2026-11-02": 14} {
		opens := On(at(t, date+NOON)).Opens
		if local := opens.In(NEW_YORK); local.Hour() != OPEN_HOUR || local.Minute() != OPEN_MINUTE || opens.UTC().Hour() != utcHour {
			t.Errorf("%s: opens %v (UTC hour %d, want %d)", date, local, opens.UTC().Hour(), utcHour)
		}
	}
	// Before the bell the next open is today's; from the bell on it is the next session's.
	if got := On(at(t, "2026-11-23 08:00")).NextOpen; !got.Equal(at(t, "2026-11-23 09:30")) {
		t.Errorf("before the open: next open %v", got)
	}
	if got := On(at(t, "2026-11-23 09:30")).NextOpen; !got.Equal(at(t, "2026-11-24 09:30")) {
		t.Errorf("at the open: next open %v", got)
	}
}

func TestAheadAndHandler(t *testing.T) {
	days := Ahead(at(t, "2026-11-21 16:30"), 7)
	if len(days) != 7 || days[0].Date != "2026-11-21" || days[6].Date != "2026-11-27" || days[5].Holiday != "Thanksgiving" {
		t.Fatalf("ahead: %d days, %s to %s", len(days), days[0].Date, days[len(days)-1].Date)
	}
	if len(Ahead(at(t, "2026-11-21 16:30"), 0)) != 1 {
		t.Error("ahead: zero days should still answer today")
	}
	for query, want := range map[string]int{"": DEFAULT_DAYS, "?days=7": 7, "?days=0": DEFAULT_DAYS, "?days=999": MAX_DAYS, "?days=soon": DEFAULT_DAYS} {
		rec := httptest.NewRecorder()
		Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/day"+query, nil))
		var body struct {
			Now  time.Time `json:"now"`
			Days []Day     `json:"days"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("%q: status %d, %v", query, rec.Code, err)
		}
		if len(body.Days) != want || body.Now.IsZero() {
			t.Errorf("%q: %d days, want %d", query, len(body.Days), want)
		}
	}
}
