package calendar

import "time"

const (
	// Structural tags: computed exactly from the exchange calendar. The keys of "tags" in strings.json.
	TAG_HOLIDAY         = "holiday"
	TAG_WEEKEND         = "weekend"
	TAG_HALF_DAY        = "half_day"
	TAG_TRIPLE_WITCHING = "triple_witching"
	TAG_MONTHLY_EXPIRY  = "monthly_expiry"
	TAG_PRE_HOLIDAY     = "pre_holiday" // the last session before the market closes for a holiday
	TAG_YEAR_END        = "year_end"
	TAG_QUARTER_END     = "quarter_end"
	TAG_MONTH_END       = "month_end"
	TAG_QUARTER_START   = "quarter_start"
	TAG_MONTH_START     = "month_start"
	TAG_VIX_EXPIRY      = "vix_expiry"
	TAG_FIRST_DAY_BACK  = "first_day_back" // the first session after a weekend or a holiday
	TAG_FRIDAY          = "friday"

	// Seasonal tags: tendencies in past data, never rules. They never lead a day's label or its playbook.
	TAG_SANTA_RALLY       = "santa_rally"
	TAG_THANKSGIVING_WEEK = "thanksgiving_week"
	TAG_EARNINGS_SEASON   = "earnings_season"
	TAG_POST_MIDTERM      = "post_midterm"
	TAG_JANUARY_EFFECT    = "january_effect"
	TAG_SEPTEMBER         = "september"
	TAG_SELL_IN_MAY       = "sell_in_may"
	TAG_SUMMER_DOLDRUMS   = "summer_doldrums"
	TAG_TAX_LOSS_SELLING  = "tax_loss_selling"
	TAG_CYCLE_POST        = "cycle_post_election"
	TAG_CYCLE_MIDTERM     = "cycle_midterm"
	TAG_CYCLE_PRE         = "cycle_pre_election"
	TAG_CYCLE_ELECTION    = "cycle_election"

	EARNINGS_START_WEEK   = 2  // reports start around the second Monday of the quarter's first month
	EARNINGS_SPAN_DAYS    = 28 // and run about four weeks
	SANTA_LAST_SESSIONS   = 5  // the last five sessions of the year
	SANTA_FIRST_SESSIONS  = 2  // and the first two of the next
	THANKSGIVING_LEAD     = -5 // the week runs from the Saturday before Thanksgiving
	THANKSGIVING_TAIL     = 1  // to the half-day Friday after it
	CYCLE_YEARS           = 4  // the US presidential cycle
	MIDTERM_REMAINDER     = 2  // year % 4 in a midterm year (2026)
	POST_MIDTERM_LAST     = time.June
	SELL_IN_MAY_FIRST     = time.May
	SELL_IN_MAY_LAST      = time.October
	TAX_LOSS_FIRST        = time.November
	SUMMER_DOLDRUMS_MONTH = time.August
)

var (
	// TAG_ORDER is every tag, most important first: a day's tags come out in this order, and the
	// first structural one names the day.
	TAG_ORDER = []string{
		TAG_HOLIDAY, TAG_WEEKEND, TAG_HALF_DAY, TAG_TRIPLE_WITCHING, TAG_MONTHLY_EXPIRY, TAG_PRE_HOLIDAY,
		TAG_YEAR_END, TAG_QUARTER_END, TAG_MONTH_END, TAG_QUARTER_START, TAG_MONTH_START, TAG_VIX_EXPIRY,
		TAG_FIRST_DAY_BACK, TAG_FRIDAY,
		TAG_SANTA_RALLY, TAG_THANKSGIVING_WEEK, TAG_EARNINGS_SEASON, TAG_POST_MIDTERM, TAG_JANUARY_EFFECT,
		TAG_SEPTEMBER, TAG_SELL_IN_MAY, TAG_SUMMER_DOLDRUMS, TAG_TAX_LOSS_SELLING,
		TAG_CYCLE_POST, TAG_CYCLE_MIDTERM, TAG_CYCLE_PRE, TAG_CYCLE_ELECTION,
	}
	// CYCLE_TAGS is the presidential cycle year, indexed by year % 4.
	CYCLE_TAGS = [CYCLE_YEARS]string{TAG_CYCLE_ELECTION, TAG_CYCLE_POST, TAG_CYCLE_MIDTERM, TAG_CYCLE_PRE}
	// EARNINGS_MONTHS are the months each quarter's reports start in.
	EARNINGS_MONTHS = []time.Month{time.January, time.April, time.July, time.October}
)

// structural marks the tags that come from the exchange calendar on d. kind is the day's session
// and holiday its holiday id. A closed day only says why it is closed: expiries and month ends
// belong to sessions.
func structural(d time.Time, kind, holiday string, has map[string]bool) {
	has[TAG_HOLIDAY] = holiday != ""
	has[TAG_WEEKEND] = holiday == "" && kind == SESSION_CLOSED
	if kind == SESSION_CLOSED {
		return
	}
	has[TAG_HALF_DAY] = kind == SESSION_HALF
	prev, next := prevTradingDay(d), nextTradingDay(d)
	quarterly := d.Month()%MONTHS_PER_QUARTER == 0
	expiry := d.Equal(monthlyExpiry(d.Year(), d.Month()))
	has[TAG_TRIPLE_WITCHING] = expiry && quarterly
	has[TAG_MONTHLY_EXPIRY] = expiry && !quarterly
	has[TAG_PRE_HOLIDAY] = closedFor(d, next) != ""
	has[TAG_VIX_EXPIRY] = d.Equal(vixExpiry(d.Year(), d.Month()))
	has[TAG_FIRST_DAY_BACK] = !prev.Equal(d.AddDate(0, 0, -1))
	has[TAG_FRIDAY] = d.Weekday() == time.Friday

	// Only the widest boundary is named: a year end is not also listed as a quarter and a month end.
	last, first := next.Month() != d.Month(), prev.Month() != d.Month()
	has[TAG_YEAR_END] = last && d.Month() == time.December
	has[TAG_QUARTER_END] = last && quarterly && !has[TAG_YEAR_END]
	has[TAG_MONTH_END] = last && !quarterly
	has[TAG_QUARTER_START] = first && d.Month()%MONTHS_PER_QUARTER == 1
	has[TAG_MONTH_START] = first && !has[TAG_QUARTER_START]
}

// seasonal marks the stretches of the year d falls in. These are tendencies people have measured
// in past prices; the engine shows them as context and never trades on them.
func seasonal(d time.Time, has map[string]bool) {
	year, month := d.Year(), d.Month()
	has[TAG_JANUARY_EFFECT] = month == time.January
	has[TAG_SEPTEMBER] = month == time.September
	has[TAG_SELL_IN_MAY] = month >= SELL_IN_MAY_FIRST && month <= SELL_IN_MAY_LAST
	has[TAG_SUMMER_DOLDRUMS] = month == SUMMER_DOLDRUMS_MONTH
	has[TAG_TAX_LOSS_SELLING] = month >= TAX_LOSS_FIRST
	has[CYCLE_TAGS[year%CYCLE_YEARS]] = true
	has[TAG_POST_MIDTERM] = year%CYCLE_YEARS == MIDTERM_REMAINDER && month >= time.November ||
		(year-1)%CYCLE_YEARS == MIDTERM_REMAINDER && month <= POST_MIDTERM_LAST

	thanksgiving := nthWeekday(year, time.November, time.Thursday, THANKSGIVING_WEEK_NO)
	has[TAG_THANKSGIVING_WEEK] = !d.Before(thanksgiving.AddDate(0, 0, THANKSGIVING_LEAD)) && !d.After(thanksgiving.AddDate(0, 0, THANKSGIVING_TAIL))

	for _, m := range EARNINGS_MONTHS {
		start := nthWeekday(year, m, time.Monday, EARNINGS_START_WEEK)
		has[TAG_EARNINGS_SEASON] = has[TAG_EARNINGS_SEASON] || !d.Before(start) && d.Before(start.AddDate(0, 0, EARNINGS_SPAN_DAYS))
	}

	// The Santa Claus rally window: from the fifth-last session of a year to the second of the next.
	from := time.Date(year+1, time.January, 1, 0, 0, 0, 0, NEW_YORK)
	for range SANTA_LAST_SESSIONS {
		from = prevTradingDay(from)
	}
	until := time.Date(year, time.January, 1, 0, 0, 0, 0, NEW_YORK).AddDate(0, 0, -1)
	for range SANTA_FIRST_SESSIONS {
		until = nextTradingDay(until)
	}
	has[TAG_SANTA_RALLY] = !d.Before(from) || !d.After(until)
}
