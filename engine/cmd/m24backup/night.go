package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	SCRATCH_PREFIX  = "m24backup-"
	NEWS_STATE_TEMP = ".state-" // a folder beside the data, where pull-backup's export builds the state
)

// schedule names the night being backed up.
type schedule struct {
	today  string         // the UTC date that names tonight's state archive
	cutoff string         // New York's date today: days before it are finished
	zone   *time.Location // New York, where a news day starts and ends
}

// nightReport says what a night did. Waiting is not an error: it lists what did not fit tonight
// and goes on a later night, in the same priority order.
type nightReport struct {
	uploaded []string
	waiting  []string
	problems []error // things that went wrong without stopping the rest of the night
}

func (r nightReport) print() {
	for _, line := range r.waiting {
		fmt.Println("waiting for a later night:", line)
	}
	for _, problem := range r.problems {
		fmt.Fprintln(os.Stderr, "m24backup: problem:", problem)
	}
	fmt.Printf("tonight sent %d archive(s); %d item(s) wait for a later night\n", len(r.uploaded), len(r.waiting))
}

// candidate is one archive the night may send.
type candidate struct {
	key     string
	label   string // what a person reads in the report
	entries []packEntry
	day     string
}

// size is what the files weigh before packing, for the report of what waits.
func (c candidate) size() int64 {
	var total int64
	for _, entry := range c.entries {
		if info, err := os.Stat(entry.path); err == nil {
			total += info.Size()
		}
	}
	return total
}

func upload(store backupStore, objects []object, key string, entries []packEntry, budget *limits) ([]object, error) {
	if budget.uploads <= 0 {
		return objects, ERR_NO_BUDGET
	}
	scratch, err := os.MkdirTemp("", SCRATCH_PREFIX)
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
	if err := pack(io.MultiWriter(file, digest), entries); err != nil {
		return objects, err
	}
	size, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return objects, err
	}
	if size > SINGLE_PUT_MAX {
		return objects, fmt.Errorf("archive exceeds the single-upload size limit")
	}
	if size > budget.night {
		return objects, fmt.Errorf("%w: %s is %d bytes, a night allows %d", ERR_TOO_BIG, key, size, budget.night)
	}
	if size > budget.bytes {
		return objects, fmt.Errorf("%w: %s is %d bytes, %d left", ERR_NO_BUDGET, key, size, budget.bytes)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return objects, err
	}
	item := object{Key: key, Size: size}
	objects, err = makeRoom(store, objects, item, budget.capBytes)
	if err != nil {
		return objects, err
	}
	if err := store.put(key, file, size, hex.EncodeToString(digest.Sum(nil))); err != nil {
		return objects, err
	}
	budget.uploads--
	budget.bytes -= size
	fmt.Printf("uploaded %s (%d bytes)\n", item.Key, item.Size)
	return append(objects, item), nil
}

// night holds one night's progress through the queues.
type night struct {
	store   backupStore
	objects []object
	budget  limits
	report  nightReport
}

// send uploads one candidate. A candidate that is over the night's budget, the bucket's cap or a
// whole night's allowance is not an error: it waits. A non-empty stop says why nothing after it in
// its queue can go tonight; a real failure (the network, R2 refusing) is returned.
func (n *night) send(item candidate) (stop string, err error) {
	objects, err := upload(n.store, n.objects, item.key, item.entries, &n.budget)
	switch {
	case err == nil:
		n.objects = objects
		n.report.uploaded = append(n.report.uploaded, item.key)
		return "", nil
	case errors.Is(err, ERR_TOO_BIG):
		n.report.waiting = append(n.report.waiting, fmt.Sprintf("%s is larger than a whole night's allowance; raise %s (up to %d bytes)",
			item.label, NIGHT_ENV, MAX_NIGHT_BYTES))
		return "", nil
	case errors.Is(err, ERR_NO_ROOM):
		return "the bucket's retention cap is full", nil
	case errors.Is(err, ERR_NO_BUDGET):
		return "tonight's bytes or uploads are used", nil
	}
	return "", fmt.Errorf("%s: %w", item.label, err)
}

// drain sends a queue in order until it stops, then reports what is left as waiting.
func (n *night) drain(name string, queue []candidate) error {
	for i, item := range queue {
		stop, err := n.send(item)
		if err != nil {
			return err
		}
		if stop != "" {
			n.report.waiting = append(n.report.waiting, describeLeft(name, stop, queue[i:]))
			return nil
		}
	}
	return nil
}

func describeLeft(name, why string, left []candidate) string {
	var bytes int64
	for _, item := range left {
		bytes += item.size()
	}
	days := make([]string, 0, len(left))
	for _, item := range left {
		if item.day != "" {
			days = append(days, item.day)
		}
	}
	sort.Strings(days)
	text := fmt.Sprintf("%d %s, about %d bytes (%s)", len(left), name, bytes, why)
	if len(days) > 0 {
		text += fmt.Sprintf(", %s to %s", days[0], days[len(days)-1])
	}
	return text
}

func fileEntries(root string, names []string) []packEntry {
	entries := make([]packEntry, len(names))
	for i, name := range names {
		entries[i] = packEntry{name: name, path: filepath.Join(root, filepath.FromSlash(name))}
	}
	return entries
}

// backup sends what fits tonight in priority order and reports what waits:
//
//  1. the current state: trading files, plus the news open day and small tables;
//  2. finished trading days, newest first;
//  3. news day files, oldest first, one object per file, so a tight night still makes progress.
//
// A day file is exported once and uploaded once. Anything over tonight's bytes, uploads or the
// retention cap waits for the next night; it never fails the sess. Only a real error (the network,
// R2 refusing an upload) does, and the news problems that did not stop the rest are returned last.
func backup(store backupStore, root string, tonight schedule, budget limits) (nightReport, error) {
	sess := &night{store: store, budget: budget}
	groups, err := archiveGroups(root, tonight.cutoff)
	if err != nil {
		return sess.report, err
	}
	sess.objects, err = store.list()
	if err != nil {
		return sess.report, err
	}
	// With no incoming object, this also applies a lower cap on the next sess.
	sess.objects, err = makeRoom(store, sess.objects, object{}, budget.capBytes)
	if err != nil {
		return sess.report, err
	}
	existing := make(map[string]bool)
	for _, item := range sess.objects {
		existing[item.Key] = true
	}
	scratch, err := os.MkdirTemp("", SCRATCH_PREFIX+"run-")
	if err != nil {
		return sess.report, err
	}
	defer os.RemoveAll(scratch)

	if _, err := exportNewsDays(root, tonight.zone, tonight.cutoff, NEWS_PART_BYTES); err != nil {
		sess.report.problems = append(sess.report.problems, fmt.Errorf("news day files: %w", err))
	}
	stateKey := OBJECT_PREFIX + STATE_GROUP + "/" + tonight.today + ARCHIVE_SUFFIX
	if !existing[stateKey] {
		entries := fileEntries(root, groups[STATE_GROUP])
		statePath := filepath.Join(scratch, NEWS_STATE_NAME)
		wrote, err := writeNewsState(root, tonight.zone, tonight.cutoff, statePath)
		if err != nil {
			sess.report.problems = append(sess.report.problems, fmt.Errorf("news state: %w", err))
		} else if wrote {
			entries = append(entries, packEntry{name: NEWS_STATE_FILE, path: statePath})
		}
		if len(entries) > 0 {
			state := candidate{key: stateKey, label: "the current state", entries: entries}
			if err := sess.drain("current state archive", []candidate{state}); err != nil {
				return sess.report, err
			}
		}
	}
	delete(groups, STATE_GROUP)

	days := make([]string, 0, len(groups))
	for day := range groups {
		days = append(days, day)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	var trading []candidate
	for _, day := range days {
		key := OBJECT_PREFIX + DAILY_KIND + "/" + day + ARCHIVE_SUFFIX
		if !existing[key] {
			trading = append(trading, candidate{key: key, label: "trading day " + day, entries: fileEntries(root, groups[day]), day: day})
		}
	}
	if err := sess.drain("trading day archive(s)", trading); err != nil {
		return sess.report, err
	}

	files, err := listNewsFiles(root)
	if err != nil {
		return sess.report, err
	}
	var newsQueue []candidate
	for _, file := range files {
		key := OBJECT_PREFIX + NEWS_KIND + "/" + file.stem() + ARCHIVE_SUFFIX
		if !existing[key] {
			newsQueue = append(newsQueue, candidate{key: key, label: "news file " + file.stem(), day: file.day,
				entries: fileEntries(root, []string{file.name})})
		}
	}
	if err := sess.drain("news day file(s)", newsQueue); err != nil {
		return sess.report, err
	}
	return sess.report, errors.Join(sess.report.problems...)
}

// exportLocal writes the news day files and a fresh state file into the data folder with no upload,
// for the PC's pull-backup, which copies them. The files belong to whoever runs it.
func exportLocal(data string, zone *time.Location, now time.Time) error {
	cutoff := now.In(zone).Format(DAY_FORMAT)
	days, err := exportNewsDays(data, zone, cutoff, NEWS_PART_BYTES)
	if err != nil {
		return err
	}
	dir := filepath.Join(data, NEWS_DIR)
	scratch, err := os.MkdirTemp(dir, NEWS_STATE_TEMP)
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	temp := filepath.Join(scratch, NEWS_STATE_NAME)
	wrote, err := writeNewsState(data, zone, cutoff, temp)
	if err != nil {
		return err
	}
	if wrote {
		if err := os.Rename(temp, filepath.Join(dir, NEWS_STATE_NAME)); err != nil {
			return err
		}
	}
	fmt.Printf("news: wrote %d day file(s)%s\n", len(days), map[bool]string{true: " and the current state", false: ""}[wrote])
	return nil
}

// rebuildLocal builds news/news.db from the day files and state file already in the folder.
func rebuildLocal(data string) error {
	if _, err := os.Stat(filepath.Join(data, filepath.FromSlash(NEWS_DB))); err == nil {
		return fmt.Errorf("%s already has a news database; rebuild into a copy of the backup folder", data)
	}
	report, err := rebuildNews(data, false)
	if err != nil {
		return err
	}
	printRebuild(report)
	return nil
}

func printRebuild(report rebuildReport) {
	if report.files == 0 {
		return
	}
	fmt.Printf("news database rebuilt from %d file(s), %d rows\n", report.files, report.rows)
	if len(report.partial) > 0 {
		fmt.Printf("warning: %d news day(s) have missing part files: %s\n", len(report.partial), listSome(report.partial))
	}
	if len(report.missing) > 0 {
		fmt.Printf("note: no news day file for %d day(s) (an empty day, or not backed up yet): %s\n",
			len(report.missing), listSome(report.missing))
	}
}

func listSome(days []string) string {
	if len(days) > MAX_MISSING_SHOW {
		return strings.Join(days[:MAX_MISSING_SHOW], ", ") + ", ..."
	}
	return strings.Join(days, ", ")
}
