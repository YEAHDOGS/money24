package predict

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// Daily published numbers: the two free pages Kalshi itself names as settlement sources.
	// AAA (gasprices.aaa.com) is the settlement source for the AAA gas and diesel series and
	// publishes the national average for regular and diesel once a day, with yesterday's and
	// last week's averages beside it. The TSA Passenger Volumes page is the settlement source
	// for the TSA series and lists every day's checkpoint count for the year so far. Neither
	// needs a key. Both update about once a day, so a half-hour cache misses nothing.
	AAA_URL         = "https://gasprices.aaa.com/"
	TSA_URL         = "https://www.tsa.gov/travel/passenger-volumes"
	DAILY_EVERY     = 30 * time.Minute
	AAA_DATE_LAYOUT = "1/2/06"
	TSA_DATE_LAYOUT = "1/2/2006"
	DATE_ONLY_HOURS = 24.0

	SOURCE_AAA_REGULAR = "aaa-regular"
	SOURCE_AAA_DIESEL  = "aaa-diesel"
	SOURCE_TSA         = "tsa"

	AAA_LABEL_CURRENT   = "Current Avg."
	AAA_LABEL_YESTERDAY = "Yesterday Avg."
	AAA_LABEL_WEEK_AGO  = "Week Ago Avg."
	AAA_COL_REGULAR     = "Regular"
	AAA_COL_DIESEL      = "Diesel"
	AAA_AS_OF           = "as of "
	DAYS_YESTERDAY      = 1
	DAYS_WEEK_AGO       = 7
)

// DailyPoint is one published value of a daily number, on its calendar date (UTC midnight).
type DailyPoint struct {
	Date  time.Time
	Value float64
}

// DailySnapshot is one source's published history, oldest first.
type DailySnapshot struct {
	At     time.Time
	Points []DailyPoint
}

// On is the published value on one date, and whether that date was published at all.
func (s *DailySnapshot) On(date time.Time) (float64, bool) {
	for _, p := range s.Points {
		if p.Date.Equal(date) {
			return p.Value, true
		}
	}
	return 0, false
}

// Latest is the newest published point on or before date.
func (s *DailySnapshot) Latest(date time.Time) (DailyPoint, bool) {
	best := DailyPoint{}
	found := false
	for _, p := range s.Points {
		if p.Date.After(date) {
			continue
		}
		if !found || p.Date.After(best.Date) {
			best, found = p, true
		}
	}
	return best, found
}

var (
	aaaRowRe  = regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	aaaCellRe = regexp.MustCompile(`(?s)<t[hd][^>]*>(.*?)</t[hd]>`)
	aaaDateRe = regexp.MustCompile(`as of (\d{1,2}/\d{1,2}/\d{2,4})`)
	tsaRowRe  = regexp.MustCompile(`<td[^>]*>\s*(\d{1,2}/\d{1,2}/\d{4})\s*</td>\s*<td[^>]*>\s*([\d,]+)\s*</td>`)
	tagRe     = regexp.MustCompile(`<[^>]+>`)
)

// getText reads one page as text. getJSON in weather_data.go is the JSON twin.
func (e *Env) getText(ctx context.Context, u string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, DATA_TIMEOUT)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", KALSHI_USER_AGENT)
	res, err := e.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, BODY_MAX))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", req.URL.Host, res.Status)
	}
	return string(body), nil
}

// parseAAA reads the AAA national-average table into regular and diesel points. The page
// gives three dated points a source: today ("Current Avg.", dated by its "as of" line),
// yesterday and a week ago. That is all the history AAA publishes for free.
func parseAAA(body string) (regular []DailyPoint, diesel []DailyPoint, err error) {
	m := aaaDateRe.FindStringSubmatch(body)
	if len(m) < 2 {
		return nil, nil, fmt.Errorf("aaa: no as-of date")
	}
	current, err := time.Parse(AAA_DATE_LAYOUT, m[1])
	if err != nil {
		return nil, nil, fmt.Errorf("aaa date: %w", err)
	}
	current = dayUTC(current)
	cols := map[string]int{}
	rows := map[string][]string{}
	for _, rowMatch := range aaaRowRe.FindAllStringSubmatch(body, -1) {
		cells := []string{}
		for _, cellMatch := range aaaCellRe.FindAllStringSubmatch(rowMatch[1], -1) {
			text := strings.TrimSpace(tagRe.ReplaceAllString(cellMatch[1], ""))
			cells = append(cells, text)
		}
		if len(cells) < 3 {
			continue
		}
		// The header is the one row naming both grades; later tables on the page (record
		// highs, state averages) reuse the grade names in other columns and must not
		// move these. Take the header once, from that row only.
		if len(cols) == 0 {
			rowCols := map[string]int{}
			for i, name := range cells {
				if name == AAA_COL_REGULAR || name == AAA_COL_DIESEL {
					rowCols[name] = i
				}
			}
			if len(rowCols) == 2 {
				cols = rowCols
			}
		}
		switch cells[0] {
		case AAA_LABEL_CURRENT, AAA_LABEL_YESTERDAY, AAA_LABEL_WEEK_AGO:
			rows[cells[0]] = cells
		}
	}
	regCol, regOK := cols[AAA_COL_REGULAR]
	dieCol, dieOK := cols[AAA_COL_DIESEL]
	if !regOK || !dieOK || len(rows) == 0 {
		return nil, nil, fmt.Errorf("aaa: averages table not found")
	}
	dates := map[string]time.Time{
		AAA_LABEL_CURRENT:   current,
		AAA_LABEL_YESTERDAY: current.AddDate(0, 0, -DAYS_YESTERDAY),
		AAA_LABEL_WEEK_AGO:  current.AddDate(0, 0, -DAYS_WEEK_AGO),
	}
	for label, cells := range rows {
		reg, regErr := aaaPrice(cells, regCol)
		die, dieErr := aaaPrice(cells, dieCol)
		if regErr != nil || dieErr != nil {
			continue
		}
		regular = append(regular, DailyPoint{Date: dates[label], Value: reg})
		diesel = append(diesel, DailyPoint{Date: dates[label], Value: die})
	}
	if len(regular) == 0 || len(diesel) == 0 {
		return nil, nil, fmt.Errorf("aaa: no averages parsed")
	}
	sortPoints(regular)
	sortPoints(diesel)
	return regular, diesel, nil
}

func aaaPrice(cells []string, col int) (float64, error) {
	if col >= len(cells) {
		return 0, fmt.Errorf("aaa: column %d missing", col)
	}
	text := strings.TrimPrefix(strings.TrimSpace(cells[col]), "$")
	return strconv.ParseFloat(text, 64)
}

// parseTSA reads the TSA Passenger Volumes table: one row a day, newest first on the page.
func parseTSA(body string) ([]DailyPoint, error) {
	points := []DailyPoint{}
	seen := map[time.Time]bool{}
	for _, m := range tsaRowRe.FindAllStringSubmatch(body, -1) {
		date, err := time.Parse(TSA_DATE_LAYOUT, m[1])
		if err != nil {
			continue
		}
		date = dayUTC(date)
		value, err := strconv.ParseFloat(strings.ReplaceAll(m[2], ",", ""), 64)
		if err != nil || seen[date] {
			continue
		}
		seen[date] = true
		points = append(points, DailyPoint{Date: date, Value: value})
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("tsa: no daily counts parsed")
	}
	sortPoints(points)
	return points, nil
}

func sortPoints(points []DailyPoint) {
	sort.Slice(points, func(i, j int) bool { return points[i].Date.Before(points[j].Date) })
}

// dayUTC is a date at UTC midnight, the calendar every daily number is dated by.
func dayUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// dailyData caches each source's published history between passes, shared by every
// daily-counts strategy so the two of them never fetch the same page twice.
type dailyData struct {
	mu        sync.Mutex
	snapshots map[string]*DailySnapshot
}

var sharedDaily = &dailyData{snapshots: map[string]*DailySnapshot{}}

// get is the cached snapshot for a source, or nil when none was ever fetched.
func (d *dailyData) get(source string) *DailySnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.snapshots[source]
}

// load fetches a source's history when its cache is stale. Both AAA columns come from
// one page, so fetching either refreshes both. A failed fetch keeps the last good cache.
func (d *dailyData) load(ctx context.Context, env *Env, source string, now time.Time) (*DailySnapshot, error) {
	d.mu.Lock()
	cached := d.snapshots[source]
	d.mu.Unlock()
	if cached != nil && now.Sub(cached.At) < DAILY_EVERY {
		return cached, nil
	}
	if source == SOURCE_AAA_REGULAR || source == SOURCE_AAA_DIESEL {
		body, err := env.getText(ctx, AAA_URL)
		if err != nil {
			return cached, err
		}
		regular, diesel, err := parseAAA(body)
		if err != nil {
			return cached, err
		}
		d.mu.Lock()
		d.snapshots[SOURCE_AAA_REGULAR] = &DailySnapshot{At: now, Points: regular}
		d.snapshots[SOURCE_AAA_DIESEL] = &DailySnapshot{At: now, Points: diesel}
		snap := d.snapshots[source]
		d.mu.Unlock()
		return snap, nil
	}
	body, err := env.getText(ctx, TSA_URL)
	if err != nil {
		return cached, err
	}
	points, err := parseTSA(body)
	if err != nil {
		return cached, err
	}
	snap := &DailySnapshot{At: now, Points: points}
	d.mu.Lock()
	d.snapshots[SOURCE_TSA] = snap
	d.mu.Unlock()
	return snap, nil
}
