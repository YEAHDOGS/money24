//go:build windows

package main

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
)

const (
	// SetThreadExecutionState flags (winbase.h). CONTINUOUS keeps the request until it is cleared;
	// SYSTEM_REQUIRED resets the idle timer, so the machine does not sleep on its own. The display
	// may still turn off, which is what a laptop left running overnight should do.
	ES_CONTINUOUS      = 0x80000000
	ES_SYSTEM_REQUIRED = 0x00000001
)

var setThreadExecutionState = syscall.NewLazyDLL("kernel32.dll").NewProc("SetThreadExecutionState")

// keepAwake stops Windows from idle-sleeping while the runner runs, until release is called or the
// process ends. The request belongs to the thread that made it, so it is made from a goroutine
// locked to its own thread that lives until release. It cannot stop a closed lid or a sleep someone
// asks for: those are set in Power Options (docs/RUNNING.md).
func keepAwake() (release func(), err error) {
	if err := setThreadExecutionState.Find(); err != nil {
		return nil, err
	}
	ready, done, cleared := make(chan error, 1), make(chan struct{}), make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(cleared)
		// The call returns the previous state, and zero only when it failed.
		if prev, _, _ := setThreadExecutionState.Call(ES_CONTINUOUS | ES_SYSTEM_REQUIRED); prev == 0 {
			ready <- errors.New("keep awake: Windows refused the request")
			return
		}
		ready <- nil
		<-done
		setThreadExecutionState.Call(ES_CONTINUOUS)
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-cleared
		})
	}, nil
}
