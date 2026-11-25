package plays

import (
	"errors"
	"time"
)

const (
	SCHEDULE_IMMEDIATE = "immediate"
	SCHEDULE_PLANNED   = "planned"
	SCHEDULE_LEGACY    = "legacy"
	PLAN_LEAD          = 24 * time.Hour
	PLAN_MIN_LEAD      = 3 * time.Hour
	PLAN_HOLD          = 24 * time.Hour
)

// immediateSchedule records the clock of an existing instant-entry strategy.
func immediateSchedule(p Play, now time.Time) Play {
	if p.ScheduleMode != "" {
		return p
	}
	p.ScheduleMode = SCHEDULE_IMMEDIATE
	p.ExpectedEntryAt = p.CreatedAt
	if p.ExpectedEntryAt.IsZero() {
		p.ExpectedEntryAt = now.UTC()
	}
	p.ExpectedExitAt = p.Failsafes.ExitBy
	if p.ExpectedExitAt.IsZero() {
		p.ExpectedExitAt = p.ExpectedEntryAt.Add(PLAN_HOLD)
		p.Failsafes.ExitBy = p.ExpectedExitAt
	}
	return p
}

// plannedSchedule prepares a manual proposal; unchanged plans may be edited when due.
func plannedSchedule(p Play, old *Play, now time.Time) (Play, error) {
	changed := old == nil || !p.ExpectedEntryAt.Equal(old.ExpectedEntryAt) || p.ScheduleMode != old.ScheduleMode
	if old != nil && old.ScheduleMode == SCHEDULE_IMMEDIATE && !changed {
		// Existing instant strategies retain their behavior when only the bracket changes.
		p.ExpectedExitAt = p.Failsafes.ExitBy
		return p, validateSchedule(p)
	}
	p.ScheduleMode = SCHEDULE_PLANNED
	if p.ExpectedEntryAt.IsZero() {
		p.ExpectedEntryAt = now.Add(PLAN_LEAD).UTC()
	}
	if changed && p.ExpectedEntryAt.Before(now.Add(PLAN_MIN_LEAD)) {
		return p, errors.New("plan entry at least three hours ahead")
	}
	if p.ExpectedExitAt.IsZero() {
		p.ExpectedExitAt = p.Failsafes.ExitBy
		if p.ExpectedExitAt.IsZero() {
			p.ExpectedExitAt = p.ExpectedEntryAt.Add(PLAN_HOLD)
		}
	}
	p.Failsafes.ExitBy = p.ExpectedExitAt
	return p, validateSchedule(p)
}

// validateSchedule checks the proposal independently of optional trading rules.
func validateSchedule(p Play) error {
	if p.ScheduleMode != SCHEDULE_IMMEDIATE && p.ScheduleMode != SCHEDULE_PLANNED {
		return errors.New("set expected entry and exit times before trading this older play")
	}
	if p.ExpectedEntryAt.IsZero() || p.ExpectedExitAt.IsZero() || !p.ExpectedExitAt.After(p.ExpectedEntryAt) {
		return errors.New("expected exit must be after expected entry; choose a later contract expiry if needed")
	}
	if !p.ExpectedExitAt.Equal(p.Failsafes.ExitBy) {
		return errors.New("the time exit must match the expected exit")
	}
	if expiry, ok := expiryClose(p.Contract.Expiry); ok && p.ExpectedExitAt.After(expiry) {
		return errors.New("expected exit must be no later than contract expiry")
	}
	return nil
}

// entryAllowed prevents early or expired entry even when paper trading bypasses rules.
func entryAllowed(p Play, now time.Time) error {
	if err := validateSchedule(p); err != nil {
		return err
	}
	if now.Before(p.ExpectedEntryAt) {
		return errors.New("expected entry has not arrived; this draft requires manual entry when due")
	}
	if !now.Before(p.ExpectedExitAt) {
		return errors.New("expected exit has passed; revise this play's schedule before entering")
	}
	return nil
}
