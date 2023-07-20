//go:build windows
// +build windows

package psutils

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows"
)

func PidExistsWithContext(ctx context.Context, pid int32) (bool, error) {
	if pid == 0 { // special case for pid 0 System Idle Process
		return true, nil
	}

	if pid < 0 {
		return false, fmt.Errorf("invalid pid %v", pid)
	}

	if pid%4 != 0 {
		// OpenProcess will succeed even on non-existing pid here https://devblogs.microsoft.com/oldnewthing/20080606-00/?p=22043
		// Valid pids are multiple of 4 so we can reject these immediately.
		return false, nil
	}

	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err == windows.ERROR_ACCESS_DENIED {
		return true, nil
	}

	if err == windows.ERROR_INVALID_PARAMETER {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	defer windows.CloseHandle(h)
	event, err := windows.WaitForSingleObject(h, 0)
	return event == uint32(windows.WAIT_TIMEOUT), err
}
