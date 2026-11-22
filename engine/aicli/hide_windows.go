//go:build windows

package aicli

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW keeps each ask from flashing a console window on the desk machine.
const CREATE_NO_WINDOW = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: CREATE_NO_WINDOW}
}
