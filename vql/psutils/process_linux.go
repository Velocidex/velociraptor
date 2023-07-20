//go:build linux || freebsd
// +build linux freebsd

package psutils

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/shirou/gopsutil/v3/process"
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
	result.Set("CreateTime", create_time)

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
