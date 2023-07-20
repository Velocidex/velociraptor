//go:build linux || freebsd || openbsd || darwin || solaris
// +build linux freebsd openbsd darwin solaris

package psutils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
)

func PidExistsWithContext(ctx context.Context, pid int32) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid pid %v", pid)
	}

	// On Posix this always succeeds so we need to check further.
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		return false, err
	}

	// Check if the proc exists
	pid_proc := GetHostProc(pid)
	_, err = os.Stat(pid_proc)
	if !os.IsNotExist(err) {
		return true, nil
	}

	// procfs does not exist or is not mounted, check PID existence by
	// signalling the pid
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false, nil
	}

	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false, err
	}

	switch errno {
	case syscall.ESRCH:
		return false, nil

	case syscall.EPERM:
		return true, nil
	}

	return false, err
}
