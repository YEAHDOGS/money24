package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// The news archive is backed up one New York day at a time. A finished day is exported once into
// news/days/YYYY-MM-DD.jsonl.gz (a day that compresses past NEWS_PART_BYTES continues in
// YYYY-MM-DD.002.jsonl.gz, .003 and so on) and uploaded once. The open day and the small current
// state travel in the nightly state archive as news/state.jsonl.gz, so that archive stays small
// however much history the database holds.
const (
	NEWS_DIR        = "news"
	NEWS_DAYS_DIR   = "news/days/"
	NEWS_STATE_FILE = "news/state.jsonl.gz"
	NEWS_STATE_NAME = "state.jsonl.gz"
	NEWS_DAY_SUFFIX = ".jsonl.gz"
	NEWS_KIND       = "news"
	NEWS_PART_FMT   = "%s.%03d" + NEWS_DAY_SUFFIX
	NEWS_PART_BYTES = 6_000_000 // compressed size where a day continues in a new part file
	NEWS_PART_WIDTH = 3
	DAY_END_LABEL   = "9999-12-31"
	NEWS_TEMP_DIR   = ".export-"
)

// NEWS_DAY_FILE matches a day file's base name: the day, then an optional part number from 002.
var NEWS_DAY_FILE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})(?:\.(\d{3}))?` + regexp.QuoteMeta(NEWS_DAY_SUFFIX) + `$`)

// newsFile is one exported day file in the data folder.
type newsFile struct {
	name string // slash path relative to the data folder
	day  string
	part int // 1 for the first file of a day
}

// stem names the file without its suffix, the way its backup object does.
func (f newsFile) stem() string {
	if f.part <= 1 {
		return f.day
	}
	return fmt.Sprintf("%s.%03d", f.day, f.part)
}

func newsPartName(day string, part int) string {
	if part <= 1 {
		return day + NEWS_DAY_SUFFIX
	}
	return fmt.Sprintf(NEWS_PART_FMT, day, part)
}

func parseNewsFile(base string) (newsFile, bool) {
	match := NEWS_DAY_FILE.FindStringSubmatch(base)
	if match == nil {
		return newsFile{}, false
	}
	if _, err := time.Parse(DAY_FORMAT, match[1]); err != nil {
		return newsFile{}, false
	}
	part := 1
	if match[2] != "" {
		part, _ = strconv.Atoi(match[2])
		if part < 2 {
			return newsFile{}, false
		}
	}
	return newsFile{name: NEWS_DAYS_DIR + base, day: match[1], part: part}, true
}

// listNewsFiles returns the day files under root, oldest day first. Unrecognised names are left alone.
func listNewsFiles(root string) ([]newsFile, error) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(NEWS_DAYS_DIR)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []newsFile
	for _, entry := range entries {
		file, ok := parseNewsFile(entry.Name())
		if ok && entry.Type().IsRegular() {
			files = append(files, file)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].day == files[j].day {
			return files[i].part < files[j].part
		}
		return files[i].day < files[j].day
	})
	return files, nil
}

// dayBounds returns the Unix milliseconds of a New York day: its start and the next day's start.
func dayBounds(zone *time.Location, day string) (start, end int64, err error) {
	first, err := time.ParseInLocation(DAY_FORMAT, day, zone)
	if err != nil {
		return 0, 0, err
	}
	return first.UnixMilli(), first.AddDate(0, 0, 1).UnixMilli(), nil
}

// dayRangeLabels lists the days from first up to, not including, end.
func dayRangeLabels(first, end string) []string {
	var days []string
	day, err := time.Parse(DAY_FORMAT, first)
	if err != nil {
		return nil
	}
	for label := first; label < end; label = day.Format(DAY_FORMAT) {
		days = append(days, label)
		day = day.AddDate(0, 0, 1)
	}
	return days
}
