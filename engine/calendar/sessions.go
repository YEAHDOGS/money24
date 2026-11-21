package calendar

import (
	"time"
	_ "time/tzdata" // New York time resolves the same on Windows, Linux and distroless
)

const (
	MARKET_TZ = "America/New_York"

	// The US stock and option session on one day.
	SESSION_OPEN   = "open"
	SESSION_HALF   = "half" // closes at 1 PM
	SESSION_CLOSED = "closed"

	OPEN_HOUR       = 9
	OPEN_MINUTE     = 30
	CLOSE_HOUR      = 16
	HALF_CLOSE_HOUR = 13

	// Holiday ids: the keys of "holidays" in strings.json.
	HOLIDAY_NEW_YEAR     = "new_year"
	HOLIDAY_MLK          = "mlk"
	HOLIDAY_WASHINGTON   = "washington"
	HOLIDAY_GOOD_FRIDAY  = "good_friday"
	HOLIDAY_MEMORIAL     = "memorial"
	HOLIDAY_JUNETEENTH   = "juneteenth"
	HOLIDAY_INDEPENDENCE = "independence"
	HOLIDAY_LABOR        = "labor"
	HOLIDAY_THANKSGIVING = "thanksgiving"
	HOLIDAY_CHRISTMAS    = "christmas"

	JUNETEENTH_SINCE     = 2022 // the first year the exchanges closed for it
	GOOD_FRIDAY_OFFSET   = -2   // days from Easter Sunday
	LAST_IN_MONTH        = -1   // a floating holiday on the last such weekday of its month
	DAYS_PER_WEEK        = 7
	CHRISTMAS_EVE_DAY    = 24
	JULY_THIRD_DAY       = 3
	THANKSGIVING_WEEK_NO = 4 // the fourth Thursday of November
	EXPIRY_WEEK_NO       = 3 // monthly options expire on the third Friday
	MONTHS_PER_QUARTER   = 3
	VIX_DAYS_BEFORE      = 30 // VIX settles this many days before the next month's third Friday

	// The anonymous Gregorian computus (Meeus, Jones, Butcher) for the date of Easter Sunday.
	// These numbers are the algorithm's own; none of them is a setting.
	METONIC_CYCLE     = 19
	CENTURY           = 100
	LEAP_CYCLE        = 4
	COMPUTUS_F_ADD    = 8
	COMPUTUS_F_DIV    = 25
	COMPUTUS_G_DIV    = 3
	COMPUTUS_H_ADD    = 15
	COMPUTUS_H_MOD    = 30
	COMPUTUS_L_ADD    = 32
	COMPUTUS_M_H      = 11
	COMPUTUS_M_L      = 22
	COMPUTUS_M_DIV    = 451
	COMPUTUS_OFFSET   = 114
	COMPUTUS_MONTH_BY = 31
)

// fixedHoliday falls on the same date every year and is moved off a weekend (see observed).
type fixedHoliday struct {
	id    string
	month time.Month
	day   int
}

// floatingHoliday is the nth such weekday of its month; nth is LAST_IN_MONTH for the last one.
type floatingHoliday struct {
	id      string
	month   time.Month
	weekday time.Weekday
	nth     int
}

var (
	// NEW_YORK is where US stock and option sessions are clocked.
	NEW_YORK = func() *time.Location {
		loc, err := time.LoadLocation(MARKET_TZ)
		if err != nil {
			panic(err)
		}
		return loc
	}()

	// The full-day holidays NYSE and Nasdaq keep, by rule (NYSE Rule 7.2), checked against the
	// exchange's published 2026 to 2028 calendar.
	FIXED_HOLIDAYS = []fixedHoliday{
		{HOLIDAY_NEW_YEAR, time.January, 1},
		{HOLIDAY_JUNETEENTH, time.June, 19},
		{HOLIDAY_INDEPENDENCE, time.July, 4},
		{HOLIDAY_CHRISTMAS, time.December, 25},
	}
	FLOATING_HOLIDAYS = []floatingHoliday{
		{HOLIDAY_MLK, time.January, time.Monday, 3},
		{HOLIDAY_WASHINGTON, time.February, time.Monday, 3},
		{HOLIDAY_MEMORIAL, time.May, time.Monday, LAST_IN_MONTH},
		{HOLIDAY_LABOR, time.September, time.Monday, 1},
		{HOLIDAY_THANKSGIVING, time.November, time.Thursday, THANKSGIVING_WEEK_NO},
	}
)

// startOfDay is midnight in New York on the day t falls on there. Every other function here
// takes days in this form, so two of them are the same day exactly when they are Equal.
func startOfDay(t time.Time) time.Time {
	year, month, day := t.In(NEW_YORK).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, NEW_YORK)
}

// nthWeekday is the nth such weekday of a month (1 is the first), or the last one for LAST_IN_MONTH.
func nthWeekday(year int, month time.Month, weekday time.Weekday, nth int) time.Time {
	if nth == LAST_IN_MONTH {
		last := time.Date(year, month+1, 0, 0, 0, 0, 0, NEW_YORK)
		return last.AddDate(0, 0, -int((last.Weekday()-weekday+DAYS_PER_WEEK)%DAYS_PER_WEEK))
	}
	first := time.Date(year, month, 1, 0, 0, 0, 0, NEW_YORK)
	return first.AddDate(0, 0, int((weekday-first.Weekday()+DAYS_PER_WEEK)%DAYS_PER_WEEK)+(nth-1)*DAYS_PER_WEEK)
}

// easter is Easter Sunday of a year in the Gregorian calendar.
func easter(year int) time.Time {
	a, b, c := year%METONIC_CYCLE, year/CENTURY, year%CENTURY
	d, e := b/LEAP_CYCLE, b%LEAP_CYCLE
	f := (b + COMPUTUS_F_ADD) / COMPUTUS_F_DIV
	g := (b - f + 1) / COMPUTUS_G_DIV
	h := (METONIC_CYCLE*a + b - d - g + COMPUTUS_H_ADD) % COMPUTUS_H_MOD
	i, k := c/LEAP_CYCLE, c%LEAP_CYCLE
	l := (COMPUTUS_L_ADD + 2*e + 2*i - h - k) % DAYS_PER_WEEK
	m := (a + COMPUTUS_M_H*h + COMPUTUS_M_L*l) / COMPUTUS_M_DIV
	n := h + l - DAYS_PER_WEEK*m + COMPUTUS_OFFSET
	return time.Date(year, time.Month(n/COMPUTUS_MONTH_BY), n%COMPUTUS_MONTH_BY+1, 0, 0, 0, 0, NEW_YORK)
}

// observed is the day the exchanges close for a fixed-date holiday: the Monday after when it
// falls on a Sunday, the Friday before when it falls on a Saturday. New Year's Day on a Saturday
// is the exception: it is not kept at all (the Friday is the year's last session), so ok is false.
func observed(id string, day time.Time) (on time.Time, ok bool) {
	switch day.Weekday() {
	case time.Sunday:
		return day.AddDate(0, 0, 1), true
	case time.Saturday:
		return day.AddDate(0, 0, -1), id != HOLIDAY_NEW_YEAR
	}
	return day, true
}

// holidayOn is the id of the full-day holiday the exchanges keep on d, or "" on any other day.
func holidayOn(d time.Time) string {
	year := d.Year()
	for _, h := range FIXED_HOLIDAYS {
		if h.id == HOLIDAY_JUNETEENTH && year < JUNETEENTH_SINCE {
			continue
		}
		if on, ok := observed(h.id, time.Date(year, h.month, h.day, 0, 0, 0, 0, NEW_YORK)); ok && on.Equal(d) {
			return h.id
		}
	}
	for _, h := range FLOATING_HOLIDAYS {
		if nthWeekday(year, h.month, h.weekday, h.nth).Equal(d) {
			return h.id
		}
	}
	if easter(year).AddDate(0, 0, GOOD_FRIDAY_OFFSET).Equal(d) {
		return HOLIDAY_GOOD_FRIDAY
	}
	return ""
}

// session is the stock and option session on d: closed on weekends and holidays, a half day
// (1 PM close) on the day after Thanksgiving, on Christmas Eve and on July 3 when those are
// trading days, and open otherwise.
func session(d time.Time) string {
	if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday || holidayOn(d) != "" {
		return SESSION_CLOSED
	}
	thanksgiving := nthWeekday(d.Year(), time.November, time.Thursday, THANKSGIVING_WEEK_NO)
	switch {
	case d.Equal(thanksgiving.AddDate(0, 0, 1)),
		d.Month() == time.December && d.Day() == CHRISTMAS_EVE_DAY,
		d.Month() == time.July && d.Day() == JULY_THIRD_DAY:
		return SESSION_HALF
	}
	return SESSION_OPEN
}

// nextTradingDay is the first day after d with a stock session.
func nextTradingDay(d time.Time) time.Time {
	for d = d.AddDate(0, 0, 1); session(d) == SESSION_CLOSED; d = d.AddDate(0, 0, 1) {
	}
	return d
}

// prevTradingDay is the last day before d with a stock session.
func prevTradingDay(d time.Time) time.Time {
	for d = d.AddDate(0, 0, -1); session(d) == SESSION_CLOSED; d = d.AddDate(0, 0, -1) {
	}
	return d
}

// closedFor is the first holiday between a trading day and the next one, or "" when only a
// plain weekend (or nothing) lies between them.
func closedFor(d, next time.Time) string {
	for day := d.AddDate(0, 0, 1); day.Before(next); day = day.AddDate(0, 0, 1) {
		if id := holidayOn(day); id != "" {
			return id
		}
	}
	return ""
}

// monthlyExpiry is the day a month's standard options expire: the third Friday, or the trading
// day before it when that Friday is a holiday.
func monthlyExpiry(year int, month time.Month) time.Time {
	friday := nthWeekday(year, month, time.Friday, EXPIRY_WEEK_NO)
	if holidayOn(friday) == "" {
		return friday
	}
	return prevTradingDay(friday)
}

// vixExpiry is the day VIX futures and options settle in a month: the Wednesday 30 days before
// the next month's third Friday, or the trading day before that Wednesday when either day is a holiday.
func vixExpiry(year int, month time.Month) time.Time {
	friday := nthWeekday(year, month+1, time.Friday, EXPIRY_WEEK_NO)
	wednesday := friday.AddDate(0, 0, -VIX_DAYS_BEFORE)
	if holidayOn(wednesday) == "" && holidayOn(friday) == "" {
		return wednesday
	}
	return prevTradingDay(wednesday)
}

// hours is the stock session on d with its open and close in New York. Both times are zero on a
// closed day.
func hours(d time.Time) (kind string, opens, closes time.Time) {
	kind = session(d)
	if kind == SESSION_CLOSED {
		return kind, time.Time{}, time.Time{}
	}
	closeHour := CLOSE_HOUR
	if kind == SESSION_HALF {
		closeHour = HALF_CLOSE_HOUR
	}
	return kind, time.Date(d.Year(), d.Month(), d.Day(), OPEN_HOUR, OPEN_MINUTE, 0, 0, NEW_YORK),
		time.Date(d.Year(), d.Month(), d.Day(), closeHour, 0, 0, 0, NEW_YORK)
}

// StocksOpen reports whether the regular US stock session is open at t: a weekday that is not a
// holiday, from 9:30 AM New York until the day's close (4 PM, or 1 PM on a half day).
func StocksOpen(t time.Time) bool {
	_, opens, closes := hours(startOfDay(t))
	return !opens.IsZero() && !t.Before(opens) && t.Before(closes)
}

// NextSettlement is when a stock or option sale becomes cash a cash account can spend again
// (T+1): 9:30 AM New York on the next trading day after the sale. Coins settle at once and never
// come through here.
func NextSettlement(sold time.Time) time.Time {
	_, opens, _ := hours(nextTradingDay(startOfDay(sold)))
	return opens
}
