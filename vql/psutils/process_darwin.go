//go:build darwin
// +build darwin

package psutils

import (
	"context"

	"golang.org/x/sys/unix"

	"github.com/Velocidex/ordereddict"
)

func GetProcess(ctx context.Context, pid int32) (*ordereddict.Dict, error) {
	proc, err := getKProc(pid)
	if err != nil {
		return nil, err
	}

	return getProcessData(proc), nil
}

func ListProcesses(ctx context.Context) ([]*ordereddict.Dict, error) {
	result := []*ordereddict.Dict{}
	processes, err := Processes()
	if err != nil {
		return nil, err
	}

	for _, item := range processes {
		result = append(result, getProcessData(item))
	}

	return result, nil
}

func Processes() ([]*unix.KinfoProc, error) {
	var ret []*unix.KinfoProc

	kprocs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return ret, err
	}

	for _, proc := range kprocs {
		ret = append(ret, &proc)
	}

	return ret, nil
}

func getKProc(pid int32) (*unix.KinfoProc, error) {
	return unix.SysctlKinfoProc("kern.proc.pid", int(pid))
}

// Only get a few fields from the process object otherwise we will
// spend too much time calling into virtual methods.
func getProcessData(proc *unix.KinfoProc) *ordereddict.Dict {
	result := ordereddict.NewDict().
		SetCaseInsensitive().
		Set("Pid", proc.Proc.P_pid).
		Set("Name", ByteToString(proc.Proc.P_comm[:])).
		Set("Ppid", proc.Proc.P_oppid).

		// Make it compatible with the Windows pslist()
		Set("CommandLine", proc.Proc.P_comm).
		Set("CreateTime", proc.Proc.P_starttime).
		Set("Times", 0).
		Set("Exe", 0).
		Set("Cwd", 0).
		Set("Username", "").
		Set("MemoryInfo", 0)

	return result
}
