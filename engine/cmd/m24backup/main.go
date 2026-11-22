// m24backup archives the Money 24 data tree to a private R2 bucket.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // New York time must resolve on a machine with no zoneinfo files

	"github.com/YEAHDOGS/money24/engine/cloudguard"
)

const (
	OBJECT_PREFIX   = "money24/v1/"
	DAILY_KIND      = "days"
	ARCHIVE_SUFFIX  = ".tar.gz"
	SINGLE_PUT_MAX  = 5_000_000_000
	HTTP_TIMEOUT    = 30 * time.Minute
	DEFAULT_DATA    = "/var/lib/money24/data"
	ACCOUNT_PATTERN = "^[a-f0-9]{32}$"
	BUCKET_PATTERN  = "^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$"
	// R2's free tier stores 10 GB and R2 has no spending cap, so the kit keeps 2 GB of headroom.
	DEFAULT_CAP = 8_000_000_000
	MAX_CAP     = 8_000_000_000
	// R2 bills writes past 1 million a month. A night makes at most MAX_LIST_PAGES + MAX_UPLOADS
	// writes (listing is a write-class operation; deletes are free).
	MAX_UPLOADS    = 8 // the state, a few trading days and news day files; still about 400 writes a month
	MAX_LIST_PAGES = 5
	// Every byte sent to R2 leaves Google, whose free tier allows 1 GB a month in all.
	DEFAULT_NIGHT_BYTES = 15_000_000
	MAX_NIGHT_BYTES     = 50_000_000
	DEFAULT_GUARD_STATE = "/var/lib/m24guard/state.json"
	CAP_ENV             = "R2_BACKUP_MAX_BYTES"
	NIGHT_ENV           = "R2_BACKUP_NIGHT_BYTES"
	MARKET_ZONE         = "America/New_York" // a day's partition is complete after New York midnight
)

var (
	ERR_NO_ROOM   = errors.New("object does not fit in the retention cap")
	ERR_NO_BUDGET = errors.New("tonight's write or egress budget is spent")
	ERR_TOO_BIG   = errors.New("archive is larger than a whole night's egress allowance")
)

// limits bound one night's run: bytes kept in the bucket, uploads made and bytes sent.
type limits struct {
	capBytes int64 // most bytes kept in the bucket
	uploads  int   // uploads left tonight
	bytes    int64 // bytes left to send tonight
	night    int64 // tonight's full allowance
}

type backupStore interface {
	list() ([]object, error)
	put(key string, data io.Reader, size int64, hash string) error
	get(key string) (io.ReadCloser, error)
	delete(key string) error
}

type backupKey struct {
	kind, day string
}

// HISTORY_KINDS are the archives named for a day and kept oldest-out; the state is the other kind.
var HISTORY_KINDS = map[string]bool{DAILY_KIND: true, NEWS_KIND: true}

func parseKey(key string) (backupKey, bool) {
	if !strings.HasPrefix(key, OBJECT_PREFIX) || !strings.HasSuffix(key, ARCHIVE_SUFFIX) {
		return backupKey{}, false
	}
	kind, name, ok := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(key, OBJECT_PREFIX), ARCHIVE_SUFFIX), "/")
	if !ok {
		return backupKey{}, false
	}
	switch kind {
	case NEWS_KIND:
		file, valid := parseNewsFile(name + NEWS_DAY_SUFFIX)
		return backupKey{kind, file.day}, valid
	case STATE_GROUP, DAILY_KIND:
		_, err := time.Parse(DAY_FORMAT, name)
		return backupKey{kind, name}, err == nil
	}
	return backupKey{}, false
}

func newestState(objects []object) string {
	var latest string
	for _, item := range objects {
		key, ok := parseKey(item.Key)
		if ok && key.kind == STATE_GROUP && item.Key > latest {
			latest = item.Key
		}
	}
	return latest
}

// retention lists oldest-first deletions and protects the latest restorable state.
func retention(objects []object, incoming object, capBytes int64) ([]object, error) {
	protected := newestState(objects)
	reserved := incoming.Size
	if reserved < 0 || reserved > capBytes {
		return nil, ERR_NO_ROOM
	}
	var candidates []object
	newKey, _ := parseKey(incoming.Key)
	for _, item := range objects {
		key, managed := parseKey(item.Key)
		if item.Size < 0 {
			return nil, fmt.Errorf("invalid size in R2 listing")
		}
		// New daily archives may replace only older history or superseded state.
		if !managed || item.Key == protected ||
			(HISTORY_KINDS[newKey.kind] && HISTORY_KINDS[key.kind] && key.day >= newKey.day) {
			if item.Size > capBytes-reserved {
				return nil, ERR_NO_ROOM
			}
			reserved += item.Size
			continue
		}
		candidates = append(candidates, item)
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, _ := parseKey(candidates[i].Key)
		right, _ := parseKey(candidates[j].Key)
		if left.day == right.day {
			return candidates[i].Key < candidates[j].Key
		}
		return left.day < right.day
	})
	// Keep the newest suffix that fits, even when an older object is small.
	for i := len(candidates) - 1; i >= 0; i-- {
		if candidates[i].Size > capBytes-reserved {
			return candidates[:i+1], nil
		}
		reserved += candidates[i].Size
	}
	return nil, nil
}

func makeRoom(store backupStore, objects []object, incoming object, capBytes int64) ([]object, error) {
	remove, err := retention(objects, incoming, capBytes)
	if err != nil {
		return objects, err
	}
	deleted := make(map[string]bool, len(remove))
	for _, item := range remove {
		if err := store.delete(item.Key); err != nil {
			return objects, err
		}
		deleted[item.Key] = true
	}
	kept := objects[:0]
	for _, item := range objects {
		if !deleted[item.Key] {
			kept = append(kept, item)
		}
	}
	return kept, nil
}

func restore(store backupStore, target string) error {
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("restore needs a new data directory with an existing parent")
	}
	objects, err := store.list()
	if err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(target), ".m24restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	state := newestState(objects)
	count := 0
	for _, item := range objects {
		key, ok := parseKey(item.Key)
		if !ok || (key.kind == STATE_GROUP && item.Key != state) {
			continue
		}
		body, err := store.get(item.Key)
		if err != nil {
			return err
		}
		err = unpack(body, staging)
		body.Close()
		if err != nil {
			return fmt.Errorf("restore %s: %w", item.Key, err)
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("no Money 24 backups found")
	}
	report, err := rebuildNews(staging, true)
	if err != nil {
		return fmt.Errorf("rebuild the news database: %w", err)
	}
	printRebuild(report)
	if err := os.Rename(staging, target); err != nil {
		return err
	}
	fmt.Printf("restored %d archive(s)\n", count)
	return nil
}

// options are the command line: one folder, one mode.
type options struct {
	data, guardFile string
	restoring       bool // download a backup into a new folder
	exportNews      bool // write the news day files and state next to the data, with no upload
	rebuildNews     bool // build news/news.db from day files and a state file already in the folder
}

func run(opts options) error {
	data, err := filepath.Abs(opts.data)
	if err != nil {
		return err
	}
	location, err := time.LoadLocation(MARKET_ZONE)
	if err != nil {
		return err
	}
	switch {
	case opts.rebuildNews:
		return rebuildLocal(data)
	case opts.exportNews:
		return exportLocal(data, location, time.Now())
	}
	account, bucket := os.Getenv("R2_ACCOUNT_ID"), os.Getenv("R2_BUCKET")
	access, secret := os.Getenv("R2_ACCESS_KEY_ID"), os.Getenv("R2_SECRET_ACCESS_KEY")
	if !regexp.MustCompile(ACCOUNT_PATTERN).MatchString(account) ||
		!regexp.MustCompile(BUCKET_PATTERN).MatchString(bucket) || access == "" || secret == "" {
		return fmt.Errorf("valid R2_ACCOUNT_ID, R2_BUCKET, R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY are required")
	}
	store := r2Store{host: account + R2_DOMAIN, bucket: bucket, access: access, secret: secret,
		client: &http.Client{Timeout: HTTP_TIMEOUT, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}}
	if opts.restoring {
		return restore(store, data)
	}
	now := time.Now()
	budget, err := nightLimits(os.Getenv, opts.guardFile, now)
	if err != nil {
		return err
	}
	fmt.Printf("tonight: up to %d uploads, %d bytes; bucket cap %d bytes\n", budget.uploads, budget.bytes, budget.capBytes)
	night := schedule{today: now.UTC().Format(DAY_FORMAT), cutoff: now.In(location).Format(DAY_FORMAT), zone: location}
	report, err := backup(store, data, night, budget)
	report.print()
	return err
}

func byteSetting(getenv func(string) string, name string, fallback, most int64) (int64, error) {
	raw := getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 || value > most {
		return 0, fmt.Errorf("%s must be a whole number of bytes from 1 to %d", name, most)
	}
	return value, nil
}

// nightLimits combines the settings with what the egress guard says is left this month.
// No guard ledger means no proof of room, so nothing is sent.
func nightLimits(getenv func(string) string, guardFile string, now time.Time) (limits, error) {
	capBytes, err := byteSetting(getenv, CAP_ENV, DEFAULT_CAP, MAX_CAP)
	if err != nil {
		return limits{}, err
	}
	night, err := byteSetting(getenv, NIGHT_ENV, DEFAULT_NIGHT_BYTES, MAX_NIGHT_BYTES)
	if err != nil {
		return limits{}, err
	}
	budget := limits{capBytes: capBytes, uploads: MAX_UPLOADS, bytes: night, night: night}
	if guardFile == "" {
		return budget, nil
	}
	state, err := cloudguard.LoadState(guardFile)
	if err != nil {
		return limits{}, err
	}
	room, err := cloudguard.BackupRoom(state, now)
	if err != nil {
		return limits{}, err
	}
	budget.bytes = min(budget.bytes, room)
	return budget, nil
}

func main() {
	opts := options{}
	flag.StringVar(&opts.data, "data", DEFAULT_DATA, "Money 24 data folder, or fresh destination for restore")
	flag.BoolVar(&opts.restoring, "restore", false, "restore the latest state and every retained day, news database included")
	flag.StringVar(&opts.guardFile, "guard-state", DEFAULT_GUARD_STATE, "egress guard ledger; empty off Google Cloud")
	flag.BoolVar(&opts.exportNews, "export-news", false, "write the news day files and state into the data folder; no upload")
	flag.BoolVar(&opts.rebuildNews, "rebuild-news", false, "build news/news.db in the data folder from its day files and state")
	flag.Parse()
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "m24backup:", err)
		os.Exit(1)
	}
}
