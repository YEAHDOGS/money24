// m24guard keeps the Money 24 machine inside Google's free tier. Every five
// minutes it meters outgoing bytes and checks the machine; it stops the
// services at the month's egress limit and refuses to start them on a machine
// Google would bill.
//
//	m24guard check   timer: meter, decide, stop or resume the services
//	m24guard allow   start gate: exit 0 only on a free machine under the limit
//	m24guard sample  meter only (at shutdown, so the last minutes are counted)
//	m24guard status  print the ledger
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/YEAHDOGS/money24/engine/cloudguard"
)

const (
	DEFAULT_STATE = "/var/lib/m24guard/state.json"
	LOCK_SUFFIX   = ".lock"
	DEFAULT_ROOT  = "/"
	READY_FILE    = "/var/lib/money24/.ready"
	ADMIN_LOCK    = "/run/lock/money24-vps.lock" // admin.sh holds it during setup, deploy and move-data
	LIMIT_ENV     = "M24GUARD_MONTH_TX_BYTES"
	GATE_UNIT     = "money24-gate.service"
	BACKUP_TIMER  = "m24backup.timer"
	BACKUP_UNIT   = "m24backup.service"
	SYSTEMCTL     = "systemctl"
	EXIT_REFUSED  = 3
)

// RESUME_UNITS start again after the guard lifts a stop. The gate starts first through their Requires=.
var RESUME_UNITS = []string{GATE_UNIT, "newsd.service", "m24d.service", "money24-cloudflared.service", BACKUP_TIMER}

type options struct {
	state, root string
	limit       int64
}

func limitBytes(flagValue int64) (int64, error) {
	limit := flagValue
	if raw := os.Getenv(LIMIT_ENV); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be a whole number of bytes", LIMIT_ENV)
		}
		limit = parsed
	}
	if limit <= 0 || limit > cloudguard.MAX_LIMIT_BYTES {
		return 0, fmt.Errorf("the monthly limit must be between 1 and %d bytes", cloudguard.MAX_LIMIT_BYTES)
	}
	return limit, nil
}

// measure locks the ledger, meters, inspects the machine and saves the result.
// decided reports whether a verdict was reached; err may still carry a failed save.
func measure(opts options, starting bool) (state cloudguard.State, verdict cloudguard.Verdict, decided bool, err error) {
	unlock, err := cloudguard.Lock(opts.state + LOCK_SUFFIX)
	if err != nil {
		return state, verdict, false, err
	}
	defer unlock()
	previous, err := cloudguard.LoadState(opts.state)
	if err != nil {
		return previous, verdict, false, err
	}
	sample, err := cloudguard.ReadSample(opts.root, time.Now())
	if err != nil {
		return previous, verdict, false, err
	}
	inspector := cloudguard.NewInspector()
	facts, factsErr := inspector.Instance()
	if factsErr == nil && cloudguard.DiskCheckDue(previous, sample) {
		// A failed disk check leaves the metadata checks standing; starting still needs one success per boot.
		if err := inspector.Disks(&facts); err != nil {
			fmt.Printf("m24guard: disk check: %v\n", err)
		}
	}
	state, verdict, err = cloudguard.Step(previous, cloudguard.Input{
		Sample: sample, LimitBytes: opts.limit, Facts: facts, FactsErr: factsErr, Starting: starting,
	})
	if err != nil {
		return previous, verdict, false, err
	}
	return state, verdict, true, cloudguard.SaveState(opts.state, state)
}

func systemctl(args ...string) {
	if output, err := exec.Command(SYSTEMCTL, args...).CombinedOutput(); err != nil {
		fmt.Printf("m24guard: systemctl %v failed: %v %s\n", args, err, output)
	}
}

func stopServices(reason string) {
	fmt.Printf("m24guard: stopping Money 24: %s\n", reason)
	systemctl("stop", BACKUP_TIMER, BACKUP_UNIT, GATE_UNIT)
}

// failClosed runs when the timer cannot meter. Without a recent count nobody knows
// how much was sent, so the services stop until counting works again.
func failClosed(opts options, cause error) error {
	state, err := cloudguard.LoadState(opts.state)
	if err == nil && time.Since(state.SampledAt) <= cloudguard.STALE_AFTER {
		return cause // one missed sample; the next run counts the gap
	}
	stopServices(fmt.Sprintf("cannot meter egress: %v", cause))
	if err == nil {
		state.Blocked, state.BlockReason = true, "cannot meter egress"
		if saveErr := cloudguard.SaveState(opts.state, state); saveErr != nil {
			fmt.Printf("m24guard: %v\n", saveErr)
		}
	}
	return cause
}

// mayResume reports whether the guard should start the services: cutover is done, no admin
// action holds the kit's lock, and the guard stopped them or the gate failed at boot.
func mayResume(verdict cloudguard.Verdict) bool {
	if _, err := os.Stat(READY_FILE); err != nil {
		return false
	}
	if cloudguard.Busy(ADMIN_LOCK) {
		fmt.Println("m24guard: an admin action is running; not starting services now")
		return false
	}
	gateDown := exec.Command(SYSTEMCTL, "is-active", "--quiet", GATE_UNIT).Run() != nil
	return verdict.Resume || gateDown
}

// check runs from the timer. Units start or stop after the ledger lock is released,
// because starting the gate runs allow, which takes the same lock. A failed save
// does not cancel the decision.
func check(opts options) error {
	state, verdict, decided, err := measure(opts, false)
	if !decided {
		return failClosed(opts, err)
	}
	switch verdict.Action {
	case cloudguard.STOP:
		stopServices(verdict.Reason)
		if verdict.PowerOff {
			fmt.Println("m24guard: powering off so Google stops billing compute and any ephemeral IPv4")
			systemctl("poweroff")
		}
	case cloudguard.HOLD:
		fmt.Printf("m24guard: holding: %s\n", verdict.Reason)
	case cloudguard.RUN:
		if mayResume(verdict) {
			fmt.Printf("m24guard: starting Money 24: %s (%d MB this month)\n", verdict.Reason, state.TxBytes/cloudguard.MEGABYTE)
			systemctl(append([]string{"start"}, RESUME_UNITS...)...)
		}
	}
	return err
}

func allow(opts options) error {
	_, verdict, decided, err := measure(opts, true)
	if !decided || err != nil {
		return fmt.Errorf("refusing to start: %w", err)
	}
	if verdict.Action != cloudguard.RUN {
		fmt.Fprintf(os.Stderr, "m24guard: refusing to start Money 24: %s\n", verdict.Reason)
		os.Exit(EXIT_REFUSED)
	}
	fmt.Printf("m24guard: %s\n", verdict.Reason)
	return nil
}

func sample(opts options) error {
	unlock, err := cloudguard.Lock(opts.state + LOCK_SUFFIX)
	if err != nil {
		return err
	}
	defer unlock()
	previous, err := cloudguard.LoadState(opts.state)
	if err != nil {
		return err
	}
	reading, err := cloudguard.ReadSample(opts.root, time.Now())
	if err != nil {
		return err
	}
	state, err := cloudguard.Advance(previous, reading)
	if err != nil {
		return err
	}
	return cloudguard.SaveState(opts.state, state)
}

func status(opts options) error {
	state, err := cloudguard.LoadState(opts.state)
	if err != nil {
		return err
	}
	room, err := cloudguard.BackupRoom(state, time.Now())
	if err != nil {
		room = 0
	}
	report := map[string]any{"ledger": state, "limit_bytes": opts.limit, "backup_room_bytes": room}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func main() {
	stateFile := flag.String("state", DEFAULT_STATE, "ledger file")
	root := flag.String("root", DEFAULT_ROOT, "filesystem root holding /sys and /proc")
	limitFlag := flag.Int64("limit", cloudguard.DEFAULT_LIMIT_BYTES, "stop the services after this many bytes sent in a month")
	flag.Parse()
	commands := map[string]func(options) error{"check": check, "allow": allow, "sample": sample, "status": status}
	command, known := commands[flag.Arg(0)]
	if !known || flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: m24guard [--limit bytes] check|allow|sample|status")
		os.Exit(2)
	}
	limit, err := limitBytes(*limitFlag)
	if err == nil {
		err = command(options{state: *stateFile, root: *root, limit: limit})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "m24guard:", err)
		os.Exit(1)
	}
}
