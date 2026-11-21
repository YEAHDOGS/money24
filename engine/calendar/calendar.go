// Package calendar labels every day the way the markets see it: which markets are open, what
// kind of day it is (a weekend, a holiday, a half day, an expiry, a month end), which stretch of
// the year it falls in, and how Money 24 treats a day like that.
//
// Everything is worked out from the date in New York time, by rule: no network, no state, no
// list of years to keep up to date. Sessions and holidays follow the exchanges' own rules and are
// exact. Seasonal tags are tendencies in past prices; they are shown as context and never traded on.
// Scheduled events (Fed, CPI, jobs, earnings) are not here: they come from newsd's calendar.
//
// The philosophy behind the labels is in docs/CALENDAR.md.
package calendar

import (
	"strings"
	"time"
)

const (
	// The markets a day is described for, in this order.
	MARKET_STOCKS     = "stocks"
	MARKET_OPTIONS    = "options"
	MARKET_FUTURES    = "futures"
	MARKET_FOREX      = "forex"
	MARKET_CRYPTO     = "crypto"
	MARKET_PREDICTION = "prediction"

	// What a market does on a day.
	STATE_OPEN   = "open"
	STATE_HALF   = "half"   // open for part of the day
	STATE_CLOSED = "closed" // shut all day
	STATE_THIN   = "thin"   // trading, on thinner books than usual
	STATE_ALWAYS = "always" // never closes

	// The kinds of day a market has a note for: the keys under each market in strings.json.
	NOTE_OPEN     = "open"
	NOTE_HALF     = "half"
	NOTE_WEEKEND  = "weekend"
	NOTE_HOLIDAY  = "holiday"
	NOTE_FRIDAY   = "friday"
	NOTE_SATURDAY = "saturday"
	NOTE_SUNDAY   = "sunday"
	NOTE_ALWAYS   = "always"
	NOTE_THIN     = "thin"

	PLAYBOOK_NORMAL = "normal" // the lines for a day nothing structural marks
	PLAYBOOK_EVENTS = "events" // where scheduled events come from; closes every weekday's playbook
	PLAYBOOK_MAX    = 4

	PLACE_HOLIDAY      = "{holiday}"
	PLACE_NEXT_HOLIDAY = "{next_holiday}"
	PLACE_NEXT_OPEN    = "{next_open}"
	DATE_LAYOUT        = "2006-01-02"
	NEXT_OPEN_LAYOUT   = "Mon Jan 2, 3:04 PM"
)

// Day is one calendar day as the markets see it.
type Day struct {
	Date     string      `json:"date"` // YYYY-MM-DD, New York
	Weekday  string      `json:"weekday"`
	Label    string      `json:"label"`   // one short line for a header chip
	Session  string      `json:"session"` // the US stock and option session: open, half or closed
	Holiday  string      `json:"holiday,omitempty"`
	Opens    time.Time   `json:"opens,omitzero"`  // regular session open; zero when closed
	Closes   time.Time   `json:"closes,omitzero"` // 4 PM New York, or 1 PM on a half day
	Markets  []MarketDay `json:"markets"`         // stocks, options, futures, forex, crypto, prediction
	Tags     []Tag       `json:"tags"`            // every label that applies, most important first
	Playbook []string    `json:"playbook"`        // how Money 24 treats this kind of day
	NextOpen time.Time   `json:"next_open"`       // the next regular stock session open: today's while it is still ahead
}

// MarketDay is what one market does on a day.
type MarketDay struct {
	Market string `json:"market"`
	State  string `json:"state"` // open, half, closed, thin or always
	Note   string `json:"note"`
}

// Tag is one label on a day, with a line saying what it means.
type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Note string `json:"note"`
}

// markets says what each market does on d. kind is the stock session and holiday its holiday id.
// Stocks and options keep exchange hours. Futures and forex run from Sunday evening to Friday
// evening, on shortened hours around US holidays. Coins and prediction markets never close, but
// coins trade on thinner books whenever the stock market is shut.
func markets(d time.Time, kind, holiday string, fill *strings.Replacer) []MarketDay {
	exchange, exchangeNote := STATE_OPEN, NOTE_OPEN
	switch {
	case holiday != "":
		exchange, exchangeNote = STATE_CLOSED, NOTE_HOLIDAY
	case kind == SESSION_CLOSED:
		exchange, exchangeNote = STATE_CLOSED, NOTE_WEEKEND
	case kind == SESSION_HALF:
		exchange, exchangeNote = STATE_HALF, NOTE_HALF
	}

	futures, forex, clockNote := STATE_OPEN, STATE_OPEN, NOTE_OPEN
	switch {
	case d.Weekday() == time.Saturday:
		futures, forex, clockNote = STATE_CLOSED, STATE_CLOSED, NOTE_SATURDAY
	case d.Weekday() == time.Sunday:
		futures, forex, clockNote = STATE_HALF, STATE_HALF, NOTE_SUNDAY
	case kind != SESSION_OPEN:
		futures, forex, clockNote = STATE_HALF, STATE_THIN, NOTE_HOLIDAY
	case d.Weekday() == time.Friday:
		clockNote = NOTE_FRIDAY
	}

	coins, coinsNote := STATE_ALWAYS, NOTE_ALWAYS
	if kind == SESSION_CLOSED {
		coins, coinsNote = STATE_THIN, NOTE_THIN
	}

	rows := []struct{ market, state, note string }{
		{MARKET_STOCKS, exchange, exchangeNote},
		{MARKET_OPTIONS, exchange, exchangeNote},
		{MARKET_FUTURES, futures, clockNote},
		{MARKET_FOREX, forex, clockNote},
		{MARKET_CRYPTO, coins, coinsNote},
		{MARKET_PREDICTION, STATE_ALWAYS, NOTE_ALWAYS},
	}
	out := make([]MarketDay, 0, len(rows))
	for _, r := range rows {
		out = append(out, MarketDay{Market: r.market, State: r.state, Note: fill.Replace(WORDING.Markets[r.market][r.note])})
	}
	return out
}

// On labels the day t falls on in New York. NextOpen is today's open while t is still before it,
// and otherwise the open of the next session.
func On(t time.Time) Day {
	d := startOfDay(t)
	holiday := holidayOn(d)
	kind, opens, closes := hours(d)
	next := opens
	if kind == SESSION_CLOSED || !t.Before(opens) {
		_, next, _ = hours(nextTradingDay(d))
	}
	fill := strings.NewReplacer(
		PLACE_HOLIDAY, WORDING.Holidays[holiday],
		PLACE_NEXT_HOLIDAY, WORDING.Holidays[closedFor(d, nextTradingDay(d))],
		PLACE_NEXT_OPEN, next.Format(NEXT_OPEN_LAYOUT),
	)
	day := Day{Date: d.Format(DATE_LAYOUT), Weekday: d.Weekday().String(), Label: WORDING.NormalLabel, Session: kind,
		Holiday: WORDING.Holidays[holiday], Opens: opens, Closes: closes, Markets: markets(d, kind, holiday, fill),
		Tags: []Tag{}, Playbook: []string{}, NextOpen: next}

	has := map[string]bool{}
	structural(d, kind, holiday, has)
	seasonal(d, has)
	// Every weekday's playbook ends on where scheduled events come from, so that line keeps its place.
	weekday := d.Weekday() != time.Saturday && d.Weekday() != time.Sunday
	room := PLAYBOOK_MAX
	if weekday {
		room -= len(WORDING.Playbook[PLAYBOOK_EVENTS])
	}
	named := false
	for _, id := range TAG_ORDER {
		if !has[id] {
			continue
		}
		text := WORDING.Tags[id]
		if !named && text.Label != "" {
			day.Label, named = fill.Replace(text.Label), true
		}
		day.Tags = append(day.Tags, Tag{ID: id, Name: text.Name, Note: fill.Replace(text.Note)})
		for _, line := range WORDING.Playbook[id] {
			if len(day.Playbook) < room {
				day.Playbook = append(day.Playbook, fill.Replace(line))
			}
		}
	}
	if len(day.Playbook) == 0 {
		day.Playbook = append(day.Playbook, WORDING.Playbook[PLAYBOOK_NORMAL]...)
	}
	if weekday {
		day.Playbook = append(day.Playbook, WORDING.Playbook[PLAYBOOK_EVENTS]...)
	}
	return day
}

// Ahead labels the day t falls on and the days after it: days of them in all, never fewer than one.
func Ahead(t time.Time, days int) []Day {
	out := []Day{On(t)}
	for i := 1; i < days; i++ {
		out = append(out, On(startOfDay(t).AddDate(0, 0, i)))
	}
	return out
}
