package memedesk

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const MIGRATION_SUFFIX = ".compact.tmp"

type legacyRecords struct {
	dir       string
	day       string
	positions map[[sha256.Size]byte]meme.Position
}

func (old *legacyRecords) lookup(row *meme.Record) (meme.Position, error) {
	day := row.At.UTC().Format(meme.DAY_LAYOUT)
	if day != old.day {
		if err := old.indexDay(day); err != nil {
			return meme.Position{}, err
		}
	}
	key, err := recordKey(row)
	return old.positions[key], err
}

func recordKey(row *meme.Record) ([sha256.Size]byte, error) {
	serialized, err := json.Marshal(row)
	return sha256.Sum256(serialized), err
}

func (old *legacyRecords) indexDay(day string) error {
	old.day, old.positions = day, make(map[[sha256.Size]byte]meme.Position)
	names, err := meme.RecordFiles(old.dir, day)
	if err != nil {
		return err
	}
	for _, name := range names {
		reader, err := meme.OpenRecords(old.dir, name)
		if err != nil {
			return err
		}
		if err := errors.Join(old.indexFile(reader), reader.Close()); err != nil {
			return err
		}
	}
	return nil
}

func (old *legacyRecords) indexFile(reader *meme.RecordReader) error {
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
		key, err := recordKey(&row)
		if err != nil {
			return err
		}
		old.positions[key] = pos
	}
}

// migrateJournal publishes the replacement only after replay and all writes
// succeed. Legacy observations with no recorder counterpart stay inline.
func migrateJournal(file *os.File, model *engine, recorderDir string) error {
	path := file.Name()
	tmp := path + MIGRATION_SUFFIX
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, JOURNAL_MODE)
	if err != nil {
		return errors.Join(err, file.Close())
	}
	defer os.Remove(tmp)
	err = rewriteJournal(file, out, model, recorderDir)
	if err := errors.Join(err, out.Sync(), out.Close(), file.Close()); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func rewriteJournal(file, out *os.File, model *engine, recorderDir string) error {
	if err := appendJournal(out, journalLine{}); err != nil {
		return err
	}
	old := legacyRecords{dir: recorderDir}
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
	if current {
		offset, err := restoreJournal(file, model, recorderDir, nil)
		if err == nil {
			if info, statErr := file.Stat(); statErr != nil {
				err = statErr
			} else if info.Size() > offset {
				err = file.Truncate(offset)
			}
		}
		if err := errors.Join(err, file.Close()); err != nil {
			return nil, err
		}
	} else if err := migrateJournal(file, model, recorderDir); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_APPEND|os.O_WRONLY, JOURNAL_MODE)
}
