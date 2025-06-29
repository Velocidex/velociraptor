//go:build linux

package throttler

import (
	"context"
	"os"
	"syscall"

	"www.velocidex.com/golang/velociraptor/vql/psutils"
)

const (
	// On Linux _SC_CLK_TCK is always 100
	// https://git.musl-libc.org/cgit/musl/tree/src/conf/sysconf.c#n33
	_SC_CLK_TCK = 100
)

type CPUReporter struct{}

func (self *CPUReporter) GetCpuTime(ctx context.Context) float64 {
	tms := &syscall.Tms{}
	_, err := syscall.Times(tms)
	if err != nil {
		return 0
	}

	total := float64(tms.Utime+tms.Stime) / _SC_CLK_TCK

	return total
}
func (self *CPUReporter) Close() {}

func NewCPUReporter() *CPUReporter {
	return &CPUReporter{}
}

type IOPSReporter struct {
	pid int32
}

func NewIOPSReporter() *IOPSReporter {
	return &IOPSReporter{
		pid: int32(os.Getpid()),
	}
}

func (self *IOPSReporter) Close() {}

func (self *IOPSReporter) GetIops(ctx context.Context) float64 {
	counters, err := psutils.IOCountersWithContext(ctx, self.pid)
	if err != nil {
		return 0
	}
	return float64(counters.ReadCount + counters.WriteCount)
}
