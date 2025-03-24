//go:build windows

package throttler

import (
	"context"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	processQueryInformation = windows.PROCESS_QUERY_LIMITED_INFORMATION
)

var (
	Modkernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessIoCounters = Modkernel32.NewProc("GetProcessIoCounters")
)

type times_t struct {
	CreateTime syscall.Filetime
	ExitTime   syscall.Filetime
	KernelTime syscall.Filetime
	UserTime   syscall.Filetime
}

type ioCounters_t struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type CPUReporter struct {
	h windows.Handle
}

func NewCPUReporter() *CPUReporter {
	// Keep the handle to the process so we can reuse it.
	h, _ := windows.OpenProcess(processQueryInformation, false, uint32(os.Getpid()))
	return &CPUReporter{h: h}
}

func (self *CPUReporter) GetCpuTime(ctx context.Context) float64 {
	var times times_t

	_ = syscall.GetProcessTimes(
		syscall.Handle(self.h),
		&times.CreateTime,
		&times.ExitTime,
		&times.KernelTime,
		&times.UserTime,
	)

	user := float64(times.UserTime.HighDateTime)*429.4967296 + float64(times.UserTime.LowDateTime)*1e-7
	kernel := float64(times.KernelTime.HighDateTime)*429.4967296 + float64(times.KernelTime.LowDateTime)*1e-7
	return user + kernel
}

func (self *CPUReporter) Close() {
	windows.CloseHandle(self.h)
}

type IOPSReporter struct {
	h windows.Handle
}

func NewIOPSReporter() *IOPSReporter {
	// Keep the handle to the process so we can reuse it.
	h, _ := windows.OpenProcess(processQueryInformation, false, uint32(os.Getpid()))
	return &IOPSReporter{h: h}
}

func (self *IOPSReporter) GetIops(ctx context.Context) float64 {
	var ioCounters ioCounters_t

	ret, _, _ := procGetProcessIoCounters.Call(uintptr(self.h),
		uintptr(unsafe.Pointer(&ioCounters)))
	if ret == 0 {
		return 0
	}

	return float64(ioCounters.ReadOperationCount + ioCounters.WriteOperationCount)
}

func (self *IOPSReporter) Close() {
	windows.CloseHandle(self.h)
}
