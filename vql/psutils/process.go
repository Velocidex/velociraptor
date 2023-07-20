package psutils

import (
	"context"
	"errors"
	"os"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/process"
)

var (
	ErrorProcessNotRunning = errors.New("ErrorProcessNotRunning")
)

type Process struct {
	Pid int32
}

func (self Process) TimesWithContext(ctx context.Context) (
	*cpu.TimesStat, error) {
	delegate := &process.Process{Pid: self.Pid}
	return delegate.TimesWithContext(ctx)
}

func (self Process) CPUPercentWithContext(ctx context.Context) (float64, error) {
	delegate := &process.Process{Pid: self.Pid}
	return delegate.CPUPercentWithContext(ctx)
}

func (self Process) IOCountersWithContext(ctx context.Context) (
	*process.IOCountersStat, error) {
	delegate := &process.Process{Pid: self.Pid}
	return delegate.IOCountersWithContext(ctx)
}

func (self Process) KillWithContext(ctx context.Context) error {
	process, err := os.FindProcess(int(self.Pid))
	if err != nil {
		return err
	}
	return process.Kill()
}

func NewProcessWithContext(ctx context.Context, pid int32) (*Process, error) {
	p := &Process{Pid: pid}

	exists, err := PidExistsWithContext(ctx, pid)
	if err != nil {
		return p, err
	}
	if !exists {
		return p, ErrorProcessNotRunning
	}
	return p, nil
}
