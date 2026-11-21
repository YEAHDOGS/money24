//go:build !windows

package main

// keepAwake does nothing here. A Linux server does not sleep, and a desktop is kept awake by what
// starts the runner: `systemd-inhibit --what=sleep m24d ...` on Linux, `caffeinate -i m24d ...` on macOS.
func keepAwake() (release func(), err error) {
	return func() {}, nil
}
