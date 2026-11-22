package meme

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	RESTORE_BUFFER = 64 << 10
	RECORD_PATTERN = FILE_PREFIX + "*" + FILE_SUFFIX
)

func (run *recording) restore(ctx context.Context, now time.Time) error {
	first := now.Add(-RECORD_WINDOW).Truncate(24 * time.Hour)
	var lastErr error
	for day := first; !day.After(now); day = day.AddDate(0, 0, 1) {
		dir := filepath.Join(run.writer.dir, day.Format(DAY_LAYOUT))
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			name := entry.Name()
			if entry.IsDir() || !strings.HasPrefix(name, FILE_PREFIX) || !strings.HasSuffix(name, FILE_SUFFIX) {
				continue
			}
			path := filepath.Join(dir, name)
			if err := run.restoreFile(ctx, path); err != nil {
				lastErr = fmt.Errorf("restore meme dataset %s: %w", filepath.Base(path), err)
			}
		}
	}
	run.tracking.prune(now)
	return lastErr
}

func (run *recording) restoreFile(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, RESTORE_BUFFER), MAX_LOOKUP_BYTES)
	var malformed error
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var row Record
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			malformed = err
			continue
		}
		run.apply(row)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return malformed
}
