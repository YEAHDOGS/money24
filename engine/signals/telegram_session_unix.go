//go:build !windows

package signals

import "os"

const TELEGRAM_SESSION_MODE = 0o600

func protectTelegramSession(file *os.File) error {
	if err := file.Chmod(TELEGRAM_SESSION_MODE); err != nil {
		return err
	}
	return nil
}
