//go:build !linux

package cloudguard

// Lock is a no-op off Linux, where the guard only runs in tests.
func Lock(string) (func(), error) {
	return func() {}, nil
}

// Busy is always false off Linux.
func Busy(string) bool {
	return false
}
