package news

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // zone names work on machines without a zone database (Windows, bare containers)
)

const (
	FF_IMPACT_HOLIDAY    = "holiday"
	FF_REGION_ALL        = "All"
	TREASURY_BILL        = "Bill"
	TREASURY_URL         = "https://www.treasurydirect.gov/auctions/upcoming/"
	TREASURY_ZONE        = "America/New_York"
	TREASURY_TIME_LAYOUT = "03:04 PM"
	BILLION              = 1e9
	CAP_HIGH             = 100 * BILLION // earnings from companies this large can move an index
	CAP_MEDIUM           = 10 * BILLION
	REGION_USD           = "USD"
	NASDAQ_EARNINGS_URL  = "https://www.nasdaq.com/market-activity/stocks/%s/earnings"
	DETAIL_SEPARATOR     = " · "
	ICS_DATE_LAYOUT      = "20060102"
	ICS_UTC_LAYOUT       = "20060102T150405Z"
	ICS_LOCAL_LAYOUT     = "20060102T150405"
	ICS_BEGIN_EVENT      = "BEGIN:VEVENT"
	ICS_END_EVENT        = "END:VEVENT"
	ICS_PARAM_TZID       = "TZID="
	ICS_DESCRIPTION_MAX  = 300
)

var (
	IMPACTS = map[string]bool{IMPACT_LOW: true, IMPACT_MEDIUM: true, IMPACT_HIGH: true}
	// When Nasdaq says a company reports, relative to the US trading session.
	NASDAQ_SESSIONS = map[string]string{"time-pre-market": "Before open", "time-after-hours": "After close"}
	MONEY_STRIPPER  = strings.NewReplacer("$", "", ",", "")
	// iCalendar folds long lines by starting the continuation with a space or tab.
	ICS_UNFOLDER  = strings.NewReplacer("\r\n ", "", "\r\n\t", "", "\n ", "", "\n\t", "")
	ICS_UNESCAPER = strings.NewReplacer(`\n`, " ", `\N`, " ", `\,`, ",", `\;`, ";", `\\`, `\`)
)

// parseFFWeek reads Forex Factory's weekly JSON export: economic releases with impact and forecasts.
func parseFFWeek(body []byte, src CalSource, _ time.Time) ([]CalEvent, error) {
	var rows []struct {
		Title    string `json:"title"`
		Country  string `json:"country"`
		Date     string `json:"date"`
		Impact   string `json:"impact"`
		Forecast string `json:"forecast"`
		Previous string `json:"previous"`
		Actual   string `json:"actual"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, err
	}
	events := []CalEvent{}
	for _, row := range rows {
		at, err := time.Parse(time.RFC3339, row.Date)
		if err != nil || strings.TrimSpace(row.Title) == "" {
			continue
		}
		e := calEvent(src, row.Date+UID_SEPARATOR+row.Country+UID_SEPARATOR+row.Title, at, false, row.Title)
		e.Forecast, e.Previous, e.Actual = row.Forecast, row.Previous, row.Actual
		if row.Country != FF_REGION_ALL {
			e.Region = row.Country
		}
		impact := strings.ToLower(row.Impact)
		if impact == FF_IMPACT_HOLIDAY {
			e.Category = CATEGORY_HOLIDAY
		}
		if IMPACTS[impact] {
			e.Impact = impact
		}
		events = append(events, e)
	}
	return events, nil
}

// parseTreasuryAuctions reads TreasuryDirect's upcoming auctions. The time is when competitive
// bidding closes, in New York. Notes and bonds matter more to rates than the weekly bills.
func parseTreasuryAuctions(body []byte, src CalSource, _ time.Time) ([]CalEvent, error) {
	var rows []struct {
		Type   string `json:"securityType"`
		Term   string `json:"securityTerm"`
		CUSIP  string `json:"cusip"`
		Amount string `json:"offeringAmount"`
		Date   string `json:"competitiveAuctionCloseDate"`
		Time   string `json:"competitiveAuctionCloseTime"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, err
	}
	zone, err := time.LoadLocation(TREASURY_ZONE)
	if err != nil {
		return nil, err
	}
	events := []CalEvent{}
	for _, row := range rows {
		if len(row.Date) < len(DAY_LAYOUT) {
			continue
		}
		date := row.Date[:len(DAY_LAYOUT)]
		at, err := time.ParseInLocation(DAY_LAYOUT+" "+TREASURY_TIME_LAYOUT, date+" "+row.Time, zone)
		allDay := err != nil
		if allDay {
			if at, err = time.Parse(DAY_LAYOUT, date); err != nil {
				continue
			}
		}
		e := calEvent(src, date+UID_SEPARATOR+row.CUSIP, at, allDay, row.Term+" "+row.Type+" auction")
		e.Region, e.URL, e.Impact = REGION_USD, TREASURY_URL, IMPACT_MEDIUM
		if row.Type == TREASURY_BILL {
			e.Impact = IMPACT_LOW
		}
		if amount, err := strconv.ParseFloat(row.Amount, 64); err == nil && amount > 0 {
			e.Detail = fmt.Sprintf("$%.0fB offered", amount/BILLION)
		}
		events = append(events, e)
	}
	return events, nil
}

// parseNasdaqEarnings reads one day of Nasdaq's earnings calendar. Companies under the source's
// min_market_cap are skipped, and impact follows company size.
func parseNasdaqEarnings(body []byte, src CalSource, day time.Time) ([]CalEvent, error) {
	var doc struct {
		Data struct {
			Rows []struct {
				Symbol    string `json:"symbol"`
				Name      string `json:"name"`
				Time      string `json:"time"`
				MarketCap string `json:"marketCap"`
				Quarter   string `json:"fiscalQuarterEnding"`
				Forecast  string `json:"epsForecast"`
				LastYear  string `json:"lastYearEPS"`
			} `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	events := []CalEvent{}
	for _, row := range doc.Data.Rows {
		marketCap, _ := strconv.ParseFloat(MONEY_STRIPPER.Replace(row.MarketCap), 64)
		if row.Symbol == "" || marketCap < src.MinMarketCap {
			continue
		}
		e := calEvent(src, day.Format(DAY_LAYOUT)+UID_SEPARATOR+row.Symbol, day, true, row.Name)
		e.Symbol, e.Region, e.Forecast, e.Previous = row.Symbol, REGION_USD, row.Forecast, row.LastYear
		e.URL = fmt.Sprintf(NASDAQ_EARNINGS_URL, url.PathEscape(strings.ToLower(row.Symbol)))
		e.Impact = IMPACT_LOW
		if marketCap >= CAP_MEDIUM {
			e.Impact = IMPACT_MEDIUM
		}
		if marketCap >= CAP_HIGH {
			e.Impact = IMPACT_HIGH
		}
		parts := []string{}
		if session := NASDAQ_SESSIONS[row.Time]; session != "" {
			parts = append(parts, session)
		}
		if row.Quarter != "" {
			parts = append(parts, "quarter ending "+row.Quarter)
		}
		if marketCap > 0 {
			parts = append(parts, fmt.Sprintf("$%.1fB cap", marketCap/BILLION))
		}
		e.Detail = strings.Join(parts, DETAIL_SEPARATOR)
		events = append(events, e)
	}
	return events, nil
}

// parseICS reads any iCalendar (.ics) feed: the open format most calendars can export. Only the
// start, summary, description, UID and URL of each VEVENT are used; repeat rules are not expanded.
func parseICS(body []byte, src CalSource, _ time.Time) ([]CalEvent, error) {
	events := []CalEvent{}
	var fields map[string]string
	var startParams string
	for _, line := range strings.Split(ICS_UNFOLDER.Replace(string(body)), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == ICS_BEGIN_EVENT {
			fields, startParams = map[string]string{}, ""
			continue
		}
		if fields == nil {
			continue
		}
		if line == ICS_END_EVENT {
			if e, ok := icsEvent(src, fields, startParams); ok {
				events = append(events, e)
			}
			fields = nil
			continue
		}
		head, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name, params, _ := strings.Cut(head, ";")
		name = strings.ToUpper(name)
		fields[name] = value
		if name == "DTSTART" {
			startParams = params
		}
	}
	return events, nil
}

func icsEvent(src CalSource, fields map[string]string, startParams string) (CalEvent, bool) {
	summary := ICS_UNESCAPER.Replace(fields["SUMMARY"])
	at, allDay, err := icsTime(fields["DTSTART"], startParams)
	if err != nil || strings.TrimSpace(summary) == "" {
		return CalEvent{}, false
	}
	e := calEvent(src, firstNonEmpty(fields["UID"], fields["DTSTART"]+UID_SEPARATOR+summary), at, allDay, summary)
	e.Detail = clean(ICS_UNESCAPER.Replace(fields["DESCRIPTION"]), ICS_DESCRIPTION_MAX)
	if u, err := url.Parse(fields["URL"]); err == nil && (u.Scheme == SCHEME_HTTP || u.Scheme == SCHEME_HTTPS) {
		e.URL = fields["URL"]
	}
	return e, true
}

// icsTime reads the three DTSTART shapes: a date (all day), UTC, or local time in a TZID zone.
// A zone this machine does not know is read as UTC.
func icsTime(value, params string) (at time.Time, allDay bool, err error) {
	value = strings.TrimSpace(value)
	if len(value) == len(ICS_DATE_LAYOUT) {
		at, err = time.Parse(ICS_DATE_LAYOUT, value)
		return at, true, err
	}
	if strings.HasSuffix(value, "Z") {
		at, err = time.Parse(ICS_UTC_LAYOUT, value)
		return at, false, err
	}
	zone := time.UTC
	for _, p := range strings.Split(params, ";") {
		name, found := strings.CutPrefix(p, ICS_PARAM_TZID)
		if !found {
			continue
		}
		if loaded, err := time.LoadLocation(name); err == nil {
			zone = loaded
		}
	}
	at, err = time.ParseInLocation(ICS_LOCAL_LAYOUT, value, zone)
	return at, false, err
}
