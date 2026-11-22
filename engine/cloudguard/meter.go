// Package cloudguard keeps the free Google Cloud machine free: it meters the
// month's outgoing bytes and checks that the machine is still a free-tier one.
package cloudguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // the billing month is Pacific time even on a machine without zoneinfo
)

const (
	// Google bills by the calendar month in US Pacific time.
	BILLING_ZONE   = "America/Los_Angeles"
	MONTH_FORMAT   = "2006-01"
	SYS_NET        = "sys/class/net"
	TX_COUNTER     = "statistics/tx_bytes"
	DEVICE_LINK    = "device" // only physical NICs have one: tunnels and WARP do not
	BOOT_ID_FILE   = "proc/sys/kernel/random/boot_id"
	STATE_MODE     = 0o644 // counters only; the backup reads them to stay under the ceiling
	STATE_DIR_MODE = 0o755
	TEMP_SUFFIX    = ".tmp"
)

// State is the guard's persisted ledger. It survives reboots, when the kernel counters restart at zero.
type State struct {
	Month         string           `json:"month"`           // billing month the total belongs to, YYYY-MM Pacific
	TxBytes       int64            `json:"tx_bytes"`        // bytes sent on physical interfaces this month
	LimitBytes    int64            `json:"limit_bytes"`     // the stop threshold in force at the last sample
	BootID        string           `json:"boot_id"`         // kernel boot the counters below belong to
	Counters      map[string]int64 `json:"counters"`        // last raw tx_bytes per interface
	SampledAt     time.Time        `json:"sampled_at"`      // time of the last sample
	Blocked       bool             `json:"blocked"`         // the guard stopped or refused the services
	BlockReason   string           `json:"block_reason"`    // why, for status and logs
	BlockMonth    string           `json:"block_month"`     // month of an egress stop, cleared when the month turns
	Strikes       int              `json:"strikes"`         // consecutive checks that saw a billable machine
	DiskBootID    string           `json:"disk_boot_id"`    // boot of the last Compute API disk check
	DiskCheckedAt time.Time        `json:"disk_checked_at"` // when it ran; a disk can grow while running
	DiskProblems  []string         `json:"disk_problems"`   // what that check found; stands until the next one
}

// Sample is one reading of the kernel's per-interface transmit counters.
type Sample struct {
	BootID   string
	Counters map[string]int64
	At       time.Time
}

// MonthKey names the billing month that contains at.
func MonthKey(at time.Time) (string, error) {
	zone, err := time.LoadLocation(BILLING_ZONE)
	if err != nil {
		return "", err
	}
	return at.In(zone).Format(MONTH_FORMAT), nil
}

// Advance adds the bytes sent since the previous sample. A new boot or a counter
// that went backwards means the kernel restarted counting, so the whole reading is new traffic.
func Advance(previous State, sample Sample) (State, error) {
	month, err := MonthKey(sample.At)
	if err != nil {
		return previous, err
	}
	next := previous
	if next.Month != month {
		next.Month = month
		next.TxBytes = 0
	}
	sameBoot := previous.BootID != "" && previous.BootID == sample.BootID
	var delta int64
	for name, counter := range sample.Counters {
		last, seen := previous.Counters[name]
		if sameBoot && seen && counter >= last {
			delta += counter - last
			continue
		}
		delta += counter
	}
	next.TxBytes += delta
	next.BootID = sample.BootID
	next.Counters = sample.Counters
	next.SampledAt = sample.At
	return next, nil
}

// ReadSample reads physical interfaces' transmit counters under root ("/" on the machine).
func ReadSample(root string, at time.Time) (Sample, error) {
	boot, err := os.ReadFile(filepath.Join(root, BOOT_ID_FILE))
	if err != nil {
		return Sample{}, fmt.Errorf("read boot id: %w", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, SYS_NET))
	if err != nil {
		return Sample{}, fmt.Errorf("list interfaces: %w", err)
	}
	counters := make(map[string]int64)
	for _, entry := range entries {
		dir := filepath.Join(root, SYS_NET, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, DEVICE_LINK)); err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, TX_COUNTER))
		if err != nil {
			return Sample{}, fmt.Errorf("read %s counter: %w", entry.Name(), err)
		}
		value, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil || value < 0 {
			return Sample{}, fmt.Errorf("invalid %s counter", entry.Name())
		}
		counters[entry.Name()] = value
	}
	if len(counters) == 0 {
		return Sample{}, errors.New("no physical network interface found")
	}
	return Sample{BootID: strings.TrimSpace(string(boot)), Counters: counters, At: at}, nil
}

// Interfaces lists the metered interfaces in a stable order, for logs.
func (sample Sample) Interfaces() []string {
	names := make([]string, 0, len(sample.Counters))
	for name := range sample.Counters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LoadState reads the ledger. A missing file is a fresh ledger.
func LoadState(file string) (State, error) {
	raw, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, fmt.Errorf("guard state is corrupt: %w", err)
	}
	return state, nil
}

// SaveState replaces the ledger atomically, so a power cut never leaves half a file.
func SaveState(file string, state State) error {
	if err := os.MkdirAll(filepath.Dir(file), STATE_DIR_MODE); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temp := file + TEMP_SUFFIX
	handle, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, STATE_MODE)
	if err != nil {
		return err
	}
	if _, err := handle.Write(raw); err != nil {
		handle.Close()
		return err
	}
	if err := handle.Sync(); err != nil {
		handle.Close()
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	return os.Rename(temp, file)
}
