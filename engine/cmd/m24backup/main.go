// m24backup archives the Money 24 data tree to a private R2 bucket.
package main

import (
	"crypto/sha256"
	"encoding/hex"
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
)

const (
	OBJECT_PREFIX   = "money24/v1/"
	DAILY_KIND      = "days"
	ARCHIVE_SUFFIX  = ".tar.gz"
	DEFAULT_CAP     = 9_000_000_000
	FREE_TIER_BYTES = 10_000_000_000
	SINGLE_PUT_MAX  = 5_000_000_000
	HTTP_TIMEOUT    = 30 * time.Minute
	DEFAULT_DATA    = "/var/lib/money24/data"
	ACCOUNT_PATTERN = "^[a-f0-9]{32}$"
	BUCKET_PATTERN  = "^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$"
)

var ERR_NO_ROOM = errors.New("object does not fit in the retention cap")

type backupStore interface {
	list() ([]object, error)
	put(key string, data io.Reader, size int64, hash string) error
	get(key string) (io.ReadCloser, error)
	delete(key string) error
}

type backupKey struct {
	kind, day string
}

func parseKey(key string) (backupKey, bool) {
	if !strings.HasPrefix(key, OBJECT_PREFIX) || !strings.HasSuffix(key, ARCHIVE_SUFFIX) {
		return backupKey{}, false
	}
	kind, day, ok := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(key, OBJECT_PREFIX), ARCHIVE_SUFFIX), "/")
	if !ok || (kind != STATE_GROUP && kind != DAILY_KIND) {
		return backupKey{}, false
	}
	_, err := time.Parse(DAY_FORMAT, day)
	return backupKey{kind, day}, err == nil
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
			(newKey.kind == DAILY_KIND && key.kind == DAILY_KIND && key.day >= newKey.day) {
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

func upload(store backupStore, objects []object, root, key string, files []string, capBytes int64) ([]object, error) {
	scratch, err := os.MkdirTemp("", "m24backup-")
	if err != nil {
		return objects, err
	}
	defer os.RemoveAll(scratch)
	file, err := os.CreateTemp(scratch, "archive-")
	if err != nil {
		return objects, err
	}
	defer file.Close()
	digest := sha256.New()
	if err := pack(io.MultiWriter(file, digest), root, files, scratch); err != nil {
		return objects, err
	}
	size, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return objects, err
	}
	if size > SINGLE_PUT_MAX {
		return objects, fmt.Errorf("archive exceeds the single-upload size limit")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return objects, err
	}
	item := object{Key: key, Size: size}
	objects, err = makeRoom(store, objects, item, capBytes)
	if err != nil {
		return objects, err
	}
	if err := store.put(key, file, size, hex.EncodeToString(digest.Sum(nil))); err != nil {
		return objects, err
	}
	fmt.Printf("uploaded %s (%d bytes)\n", item.Key, item.Size)
	return append(objects, item), nil
}

func backup(store backupStore, root, today, cutoff string, capBytes int64) error {
	groups, err := archiveGroups(root, cutoff)
	if err != nil {
		return err
	}
	objects, err := store.list()
	if err != nil {
		return err
	}
	// With no incoming object, this also applies a lower cap on the next run.
	objects, err = makeRoom(store, objects, object{}, capBytes)
	if err != nil {
		return err
	}
	existing := make(map[string]bool)
	for _, item := range objects {
		existing[item.Key] = true
	}
	stateKey := OBJECT_PREFIX + STATE_GROUP + "/" + today + ARCHIVE_SUFFIX
	if len(groups[STATE_GROUP]) > 0 && !existing[stateKey] {
		objects, err = upload(store, objects, root, stateKey, groups[STATE_GROUP], capBytes)
		if err != nil {
			return fmt.Errorf("current state: %w", err)
		}
	}
	delete(groups, STATE_GROUP)
	days := make([]string, 0, len(groups))
	for day := range groups {
		days = append(days, day)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	for _, day := range days {
		key := OBJECT_PREFIX + DAILY_KIND + "/" + day + ARCHIVE_SUFFIX
		if existing[key] {
			continue
		}
		objects, err = upload(store, objects, root, key, groups[day], capBytes)
		if errors.Is(err, ERR_NO_ROOM) {
			fmt.Printf("retention cap reached at %s\n", day)
			return nil
		}
		if err != nil {
			return fmt.Errorf("archive %s: %w", day, err)
		}
	}
	return nil
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
	if err := os.Rename(staging, target); err != nil {
		return err
	}
	fmt.Printf("restored %d archive(s)\n", count)
	return nil
}

func run(data string, restoring bool) error {
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
	data, err := filepath.Abs(data)
	if err != nil {
		return err
	}
	if restoring {
		return restore(store, data)
	}
	capBytes := int64(DEFAULT_CAP)
	if raw := os.Getenv("R2_BACKUP_MAX_BYTES"); raw != "" {
		capBytes, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || capBytes <= 0 || capBytes >= FREE_TIER_BYTES {
			return fmt.Errorf("R2_BACKUP_MAX_BYTES must be positive and below 10000000000")
		}
	}
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		return err
	}
	now := time.Now()
	return backup(store, data, now.UTC().Format(DAY_FORMAT), now.In(location).Format(DAY_FORMAT), capBytes)
}

func main() {
	data := flag.String("data", DEFAULT_DATA, "Money 24 data folder, or fresh destination for restore")
	restoring := flag.Bool("restore", false, "restore the latest state and all retained days")
	flag.Parse()
	if err := run(*data, *restoring); err != nil {
		fmt.Fprintln(os.Stderr, "m24backup:", err)
		os.Exit(1)
	}
}
