//go:build !windows

package aicli

import "os/exec"

func hideWindow(*exec.Cmd) {}
