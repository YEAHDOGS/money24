//go:build linux

package cloudguard

import (
	"os"
	"syscall"
)

const LOCK_MODE = 0o600

// Lock holds an exclusive lock on file until the returned function runs.
// The timer and the start gate never write the ledger at the same time.
func Lock(file string) (func(), error) {
	handle, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, LOCK_MODE)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_EX); err != nil {
		handle.Close()
		return nil, err
	}
	return func() { handle.Close() }, nil
}
