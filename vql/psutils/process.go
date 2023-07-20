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
	ErrorNotPermitted                     = errors.New("operation not permitted")
)

type TimesStat struct {
		CPU       string  `json:"cpu"`
		User      float64 `json:"user"`
		System    float64 `json:"system"`
		Idle      float64 `json:"idle"`
		Nice      float64 `json:"nice"`
		Iowait    float64 `json:"iowait"`
		Irq       float64 `json:"irq"`
		Softirq   float64 `json:"softirq"`
		Steal     float64 `json:"steal"`
		Guest     float64 `json:"guest"`
		GuestNice float64 `json:"guestNice"`
}

type MemoryInfoStat struct {
	RSS    uint64 `json:"rss"`    // bytes
	VMS    uint64 `json:"vms"`    // bytes
	HWM    uint64 `json:"hwm"`    // bytes
	Data   uint64 `json:"data"`   // bytes
	Stack  uint64 `json:"stack"`  // bytes
	Locked uint64 `json:"locked"` // bytes
	Swap   uint64 `json:"swap"`   // bytes
}

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
