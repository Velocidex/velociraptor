//go:build windows
// +build windows

package psutils

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/shirou/gopsutil/v4/host"
	"golang.org/x/sys/windows"
)

const (
	processQueryInformation = windows.PROCESS_QUERY_LIMITED_INFORMATION
)

var (
	Modkernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessIoCounters = Modkernel32.NewProc("GetProcessIoCounters")

	modpsapi                 = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemoryInfo = modpsapi.NewProc("GetProcessMemoryInfo")
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

func TimesWithContext(ctx context.Context, pid int32) (*TimesStat, error) {
	var times struct {
		CreateTime syscall.Filetime
		ExitTime   syscall.Filetime
		KernelTime syscall.Filetime
		UserTime   syscall.Filetime
	}

	h, err := windows.OpenProcess(processQueryInformation, false, uint32(pid))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)

	err = syscall.GetProcessTimes(
		syscall.Handle(h),
		&times.CreateTime,
		&times.ExitTime,
		&times.KernelTime,
		&times.UserTime,
	)

	user := float64(times.UserTime.HighDateTime)*429.4967296 + float64(times.UserTime.LowDateTime)*1e-7
	kernel := float64(times.KernelTime.HighDateTime)*429.4967296 + float64(times.KernelTime.LowDateTime)*1e-7

	return &TimesStat{
		User:   user,
		System: kernel,
	}, nil
}

func getProcessMemoryInfo(h windows.Handle, mem *PROCESS_MEMORY_COUNTERS) (err error) {
	r1, _, e1 := syscall.Syscall(procGetProcessMemoryInfo.Addr(), 3, uintptr(h), uintptr(unsafe.Pointer(mem)), uintptr(unsafe.Sizeof(*mem)))
	if r1 == 0 {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func MemoryInfoWithContext(ctx context.Context, pid int32) (*MemoryInfoStat, error) {
	var mem PROCESS_MEMORY_COUNTERS

	c, err := windows.OpenProcess(processQueryInformation, false, uint32(pid))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(c)
	if err := getProcessMemoryInfo(c, &mem); err != nil {
		return nil, err
	}

	ret := &MemoryInfoStat{
		RSS: uint64(mem.WorkingSetSize),
		VMS: uint64(mem.PagefileUsage),
	}

	return ret, nil
}

func IOCountersWithContext(ctx context.Context, pid int32) (*IOCountersStat, error) {
	// ioCounters is an equivalent representation of IO_COUNTERS in the Windows API.
	// https://docs.microsoft.com/windows/win32/api/winnt/ns-winnt-io_counters
	var ioCounters struct {
		ReadOperationCount  uint64
		WriteOperationCount uint64
		OtherOperationCount uint64
		ReadTransferCount   uint64
		WriteTransferCount  uint64
		OtherTransferCount  uint64
	}

	c, err := windows.OpenProcess(processQueryInformation, false, uint32(pid))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(c)

	ret, _, err := procGetProcessIoCounters.Call(uintptr(c), uintptr(unsafe.Pointer(&ioCounters)))
	if ret == 0 {
		return nil, err
	}
	stats := &IOCountersStat{
		ReadCount:  ioCounters.ReadOperationCount,
		ReadBytes:  ioCounters.ReadTransferCount,
		WriteCount: ioCounters.WriteOperationCount,
		WriteBytes: ioCounters.WriteTransferCount,
	}

	return stats, nil
}

// Pretty cheap as it is just a reg lookup
func HostID() string {
	id, _ := host.HostID()
	return id
}
