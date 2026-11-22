package memedesk

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	MIGRATION_SUFFIX = ".compact.tmp"
	BACKUP_SUFFIX    = ".v1.bak" // the legacy journal, kept until its replacement replays to the same desk
)

var errMigrationDiverged = errors.New("migrated journal does not replay to the same desk; the legacy journal is untouched")

type recordKey [sha256.Size]byte

// legacyRecords finds the recorder line that holds the same observation as a
// legacy journal copy, one UTC day of the recorder at a time.
type legacyRecords struct {
	dir  string
	days map[string]map[recordKey]meme.Position
}

func (old *legacyRecords) lookup(row *meme.Record) (meme.Position, error) {
	day := row.At.UTC().Format(meme.DAY_LAYOUT)
	positions, ok := old.days[day]
	if !ok {
		var err error
		if positions, err = indexDay(old.dir, day); err != nil {
			return meme.Position{}, err
		}
		old.days[day] = positions
	}
	key, err := keyOf(row)
	return positions[key], err
}

// keyOf hashes the observation as the desk sees it: both copies are decoded
// into the same struct, so only fields the desk reads take part.
func keyOf(row *meme.Record) (recordKey, error) {
	serialized, err := json.Marshal(row)
	return sha256.Sum256(serialized), err
}

func indexDay(dir, day string) (map[recordKey]meme.Position, error) {
	positions := make(map[recordKey]meme.Position)
	names, err := meme.RecordFiles(dir, day)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		reader, err := meme.OpenRecords(dir, name)
		if err != nil {
			return nil, err
		}
		if err := errors.Join(indexFile(reader, positions), reader.Close()); err != nil {
			return nil, err
		}
	}
	return positions, nil
}

func indexFile(reader *meme.RecordReader, positions map[recordKey]meme.Position) error {
	for {
		line, pos, err := reader.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, meme.ErrIncompleteRecord) {
			return nil
		}
		if err != nil {
			return err
		}
		var row meme.Record
		if err := json.Unmarshal(line, &row); err != nil {
			continue
		}
		key, err := keyOf(&row)
		if err != nil {
			return err
		}
		// Identical observations are interchangeable; the first copy is enough.
		if _, seen := positions[key]; !seen {
			positions[key] = pos
		}
	}
}

// migrateJournal replaces a legacy journal with recorder references. It backs
// the original up first, replays the replacement into a fresh desk, and
// publishes it only if that desk equals the one the legacy journal built.
// Legacy observations with no recorder counterpart stay inline.
func migrateJournal(file *os.File, model *engine, recorderDir string) error {
	path := file.Name()
	backup := path + BACKUP_SUFFIX
	if err := backupJournal(file, backup); err != nil {
		return errors.Join(err, file.Close())
	}
	tmp := path + MIGRATION_SUFFIX
	defer os.Remove(tmp)
	if err := errors.Join(writeMigration(file, tmp, model, recorderDir), file.Close()); err != nil {
		return err
	}
	if err := verifyMigration(tmp, model, recorderDir); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return removeBackup(backup)
}

// removeBackup deletes the legacy copy once a verified migration is in place;
// a new desk's empty journal never had one.
func removeBackup(backup string) error {
	if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func backupJournal(file *os.File, backup string) error {
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return err
	}
	if _, err := os.Stat(backup); err == nil {
		// An earlier attempt stopped before publishing; its backup is this same journal.
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp := backup + MIGRATION_SUFFIX
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, JOURNAL_MODE)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	_, copyErr := io.Copy(out, file)
	_, seekErr := file.Seek(0, io.SeekStart)
	if err := errors.Join(copyErr, seekErr, out.Sync(), out.Close()); err != nil {
		return err
	}
	return os.Rename(tmp, backup)
}

func writeMigration(file *os.File, tmp string, model *engine, recorderDir string) error {
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, JOURNAL_MODE)
	if err != nil {
		return err
	}
	err = rewriteJournal(file, out, model, recorderDir)
	return errors.Join(err, out.Sync(), out.Close())
}

func rewriteJournal(file, out *os.File, model *engine, recorderDir string) error {
	if err := appendJournal(out, journalLine{}); err != nil {
		return err
	}
	old := legacyRecords{dir: recorderDir, days: make(map[string]map[recordKey]meme.Position)}
	var last meme.Position
	_, err := restoreJournal(file, model, recorderDir, func(line journalLine, raw []byte) error {
		if line.Record == nil {
			return appendJournal(out, line)
		}
		pos, err := old.lookup(line.Record)
		if err != nil {
			return err
		}
		if pos.File == "" {
			_, err := out.Write(raw)
			return err
		}
		return appendPosition(out, &last, pos)
	})
	return err
}

func verifyMigration(tmp string, want *engine, recorderDir string) error {
	file, err := os.Open(tmp)
	if err != nil {
		return err
	}
	got := newEngine()
	_, err = restoreJournal(file, got, recorderDir, nil)
	if err := errors.Join(err, file.Close()); err != nil {
		return fmt.Errorf("verify migrated journal: %w", err)
	}
	if !reflect.DeepEqual(got, want) {
		return errMigrationDiverged
	}
	return nil
}

func openJournal(dataDir string, model *engine) (*os.File, error) {
	path := filepath.Join(dataDir, JOURNAL_FILE)
	recorderDir := filepath.Join(dataDir, meme.DATA_SUBDIR)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, JOURNAL_MODE)
	if err != nil {
		return nil, err
	}
	current, err := currentJournal(file)
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if !current {
		if err := migrateJournal(file, model, recorderDir); err != nil {
			return nil, fmt.Errorf("migrate meme desk journal: %w", err)
		}
		return os.OpenFile(path, os.O_APPEND|os.O_WRONLY, JOURNAL_MODE)
	}
	offset, err := restoreJournal(file, model, recorderDir, nil)
	if err == nil {
		err = truncateTail(file, offset)
	}
	if err := errors.Join(err, file.Close()); err != nil {
		return nil, err
	}
	// A published migration verified before it replaced the legacy journal.
	if err := removeBackup(path + BACKUP_SUFFIX); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_APPEND|os.O_WRONLY, JOURNAL_MODE)
}

func truncateTail(file *os.File, offset int64) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() <= offset {
		return nil
	}
	return file.Truncate(offset)
}
