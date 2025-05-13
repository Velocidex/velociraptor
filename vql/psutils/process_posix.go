//go:build linux || freebsd
// +build linux freebsd

// This file is for operating systems with /proc

package psutils

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/process"
)

func GetProcess(ctx context.Context, pid int32) (*ordereddict.Dict, error) {
	process_obj, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil, err
	}

	return getProcessData(process_obj), nil
}

func ListProcesses(ctx context.Context) ([]*ordereddict.Dict, error) {
	result := []*ordereddict.Dict{}
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	for _, item := range processes {
		result = append(result, getProcessData(item))
	}

	return result, nil
}

// Only get a few fields from the process object otherwise we will
// spend too much time calling into virtual methods.
func getProcessData(process *process.Process) *ordereddict.Dict {
	result := ordereddict.NewDict().SetCaseInsensitive().
		Set("Pid", process.Pid)

	name, _ := process.Name()
	result.Set("Name", name)

	ppid, _ := process.Ppid()
	result.Set("Ppid", ppid)

	// Make it compatible with the Windows pslist()
	cmdline, _ := process.Cmdline()
	result.Set("CommandLine", cmdline)

	create_time, _ := process.CreateTime()
	create_time_string := time.Unix(create_time/1000, create_time%1000).
		Format(time.RFC3339)
	result.Set("CreateTime", create_time_string)

	times, _ := process.Times()
	result.Set("Times", times)

	exe, _ := process.Exe()
	result.Set("Exe", exe)

	cwd, _ := process.Cwd()
	result.Set("Cwd", cwd)

	user, _ := process.Username()
	result.Set("Username", user)

	memory_info, _ := process.MemoryInfo()
	result.Set("MemoryInfo", memory_info)

	return result
}

func MemoryInfoWithContext(ctx context.Context, pid int32) (*MemoryInfoStat, error) {
	delegate := &process.Process{Pid: pid}
	mem, err := delegate.MemoryInfoWithContext(ctx)
	if err != nil {
		return nil, err
	}

	ret := &MemoryInfoStat{
		RSS:  mem.RSS,
		VMS:  mem.VMS,
		Swap: mem.Swap,
	}

	return ret, nil
}

func TimesWithContext(ctx context.Context, pid int32) (*TimesStat, error) {
	delegate := &process.Process{Pid: pid}
	times, err := delegate.TimesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	return &TimesStat{
		User:   times.User,
		System: times.System,
	}, nil
}

func IOCountersWithContext(ctx context.Context, pid int32) (*IOCountersStat, error) {
	delegate := &process.Process{Pid: pid}
	counters, err := delegate.IOCountersWithContext(ctx)

	if err != nil {
		return nil, err
	}

	return &IOCountersStat{
		ReadCount:  counters.ReadCount,
		WriteCount: counters.WriteCount,
		ReadBytes:  counters.ReadBytes,
		WriteBytes: counters.WriteBytes,
	}, nil
}

// Pretty cheap as it is just a /proc read.
func HostID() string {
	id, _ := host.HostID()
	return id
}
