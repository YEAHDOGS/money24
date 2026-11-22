package meme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

func (run *recording) restore(ctx context.Context, now time.Time) error {
	first := now.Add(-RECORD_WINDOW).Truncate(24 * time.Hour)
	var lastErr error
	for day := first; !day.After(now); day = day.AddDate(0, 0, 1) {
		files, err := RecordFiles(run.writer.dir, day.Format(DAY_LAYOUT))
		if err != nil {
			return err
		}
		for _, name := range files {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := run.restoreFile(ctx, name); err != nil {
				lastErr = fmt.Errorf("restore meme dataset %s: %w", name, err)
			}
		}
	}
	run.tracking.prune(now)
	return lastErr
}

func (run *recording) restoreFile(ctx context.Context, name string) error {
	reader, err := OpenRecords(run.writer.dir, name)
	if err != nil {
		return err
	}
	defer reader.Close()
	var malformed error
	for ctx.Err() == nil {
		line, _, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return malformed
		}
		if err != nil {
			return err
		}
		var row Record
		if err := json.Unmarshal(line, &row); err != nil {
			malformed = err
			continue
		}
		run.apply(row)
	}
	return ctx.Err()
}
