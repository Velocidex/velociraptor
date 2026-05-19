//go:build !windows
// +build !windows

package shell

import (
	"os/exec"
	"syscall"
)

// For posix systems we want the child to run in a different process
// group so it can not capture our signals.
func UpdateCommandForOS(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
