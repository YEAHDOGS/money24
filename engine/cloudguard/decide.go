package cloudguard

import (
	"fmt"
	"strings"
	"time"
)

const (
	// Google's free tier allows 1 GB of North American egress a month; stop well short of it.
	DEFAULT_LIMIT_BYTES = 800_000_000
	MAX_LIMIT_BYTES     = 950_000_000
	// Two checks in a row must see a billable machine before the guard powers it off.
	STRIKES_TO_POWER_OFF = 2
	// The disk type and size come from the Compute API; check them each boot and hourly after.
	DISK_RECHECK = time.Hour
	// Backups only move data while the month's total is below this share of the limit.
	BACKUP_SHARE_PERCENT = 75
	// A ledger older than this means the guard is not running; backups and the timer treat it as unknown.
	STALE_AFTER = 30 * time.Minute
	MEGABYTE    = 1_000_000
)

// Action is what the services should do now.
type Action int

const (
	RUN  Action = iota // free and under the limit
	STOP               // over the limit or on a billable machine
	HOLD               // facts unreadable: leave running services alone, start nothing new
)

// Verdict is one decision with its reason, ready for the journal.
type Verdict struct {
	Action   Action
	Reason   string
	PowerOff bool // a billable machine was seen twice in a row: power off to stop compute and IPv4 charges
	Resume   bool // the guard had stopped the services and they may run again
}

// Input is everything one check knows.
type Input struct {
	Sample     Sample
	LimitBytes int64
	Facts      Facts
	FactsErr   error // nil when the metadata server and, if due, the Compute API answered
	Starting   bool  // the services are about to start: refuse unless certain
}

// DiskCheckDue reports whether the Compute API disk check must run on this sample.
func DiskCheckDue(state State, sample Sample) bool {
	return state.DiskBootID != sample.BootID || sample.At.Sub(state.DiskCheckedAt) >= DISK_RECHECK
}

// Step meters the sample, decides, and returns the ledger to save with the verdict.
func Step(previous State, in Input) (State, Verdict, error) {
	state, err := Advance(previous, in.Sample)
	if err != nil {
		return previous, Verdict{}, err
	}
	state.LimitBytes = in.LimitBytes
	if in.FactsErr == nil && in.Facts.DiskChecked {
		state.DiskBootID = in.Sample.BootID
		state.DiskCheckedAt = in.Sample.At
		state.DiskProblems = diskViolations(in.Facts)
	}
	// Disk problems found by the last Compute API check stand until the next one.
	var found []string
	if in.FactsErr == nil {
		found = append(machineViolations(in.Facts), state.DiskProblems...)
	}
	verdict := decide(state, in, found)
	switch verdict.Action {
	case STOP:
		state.Blocked, state.BlockReason = true, verdict.Reason
		if len(found) > 0 {
			state.Strikes++
			verdict.PowerOff = state.Strikes >= STRIKES_TO_POWER_OFF
		}
	case RUN:
		verdict.Resume = state.Blocked
		state.Blocked, state.BlockReason, state.Strikes = false, "", 0
	case HOLD:
		if in.Starting {
			state.Blocked, state.BlockReason = true, verdict.Reason
		}
	}
	return state, verdict, nil
}

func decide(state State, in Input, found []string) Verdict {
	if state.TxBytes >= in.LimitBytes {
		return Verdict{Action: STOP, Reason: fmt.Sprintf("sent %d MB this month (%s Pacific), the limit is %d MB",
			state.TxBytes/MEGABYTE, state.Month, in.LimitBytes/MEGABYTE)}
	}
	if in.FactsErr != nil {
		return Verdict{Action: HOLD, Reason: "cannot confirm a free-tier machine: " + in.FactsErr.Error()}
	}
	if len(found) > 0 {
		return Verdict{Action: STOP, Reason: "billable machine: " + strings.Join(found, "; ")}
	}
	if in.Starting && !in.Facts.DiskChecked && state.DiskBootID != in.Sample.BootID {
		return Verdict{Action: HOLD, Reason: "the boot disk has not been checked on this boot"}
	}
	return Verdict{Action: RUN, Reason: fmt.Sprintf("free-tier machine, %d of %d MB sent this month",
		state.TxBytes/MEGABYTE, in.LimitBytes/MEGABYTE)}
}

// BackupRoom is how many more bytes a backup may send this month without
// pushing the month's total past BACKUP_SHARE_PERCENT of the guard's limit.
// A ledger the guard has not updated recently proves nothing, so it leaves no room.
func BackupRoom(state State, now time.Time) (int64, error) {
	month, err := MonthKey(now)
	if err != nil {
		return 0, err
	}
	if state.LimitBytes <= 0 {
		return 0, fmt.Errorf("the egress guard has not sampled yet")
	}
	if now.Sub(state.SampledAt) > STALE_AFTER {
		return 0, fmt.Errorf("the egress guard last sampled at %s", state.SampledAt.Format(time.RFC3339))
	}
	used := state.TxBytes
	if state.Month != month {
		used = 0
	}
	room := state.LimitBytes*BACKUP_SHARE_PERCENT/100 - used
	return max(room, 0), nil
}
