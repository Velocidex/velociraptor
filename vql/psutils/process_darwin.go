//go:build darwin
// +build darwin

package psutils

import (
	"context"
	"os/user"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Velocidex/ordereddict"
)

func GetProcess(ctx context.Context, pid int32) (*ordereddict.Dict, error) {
	proc, err := getKProc(pid)
	if err != nil {
		return nil, err
	}

	return getProcessData(ctx, proc), nil
}

func ListProcesses(ctx context.Context) ([]*ordereddict.Dict, error) {
	result := []*ordereddict.Dict{}
	processes, err := Processes()
	if err != nil {
		return nil, err
	}

	for _, item := range processes {
		result = append(result, getProcessData(ctx, &item))
	}

	return result, nil
}

func Processes() ([]unix.KinfoProc, error) {
	var ret []unix.KinfoProc

	kprocs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return ret, err
	}

	for _, proc := range kprocs {
		ret = append(ret, proc)
	}

	return ret, nil
}

func getKProc(pid int32) (*unix.KinfoProc, error) {
	return unix.SysctlKinfoProc("kern.proc.pid", int(pid))
}

func getProcessData(ctx context.Context,
	proc *unix.KinfoProc) *ordereddict.Dict {
	pid := proc.Proc.P_pid

	name, err := cmdNameWithContext(ctx, pid)
	if err != nil {
		name = ByteToString(proc.Proc.P_comm[:])
	}

	cmdline, err := cmdlineSliceWithContext(ctx, pid)
	if err != nil {
		cmdline = append(cmdline, name)
	}

	times, _ := TimesWithContext(ctx, pid)
	exe, _ := ExeWithContext(ctx, pid)
	cwd, _ := CwdWithContext(ctx, pid)

	uid := proc.Eproc.Ucred.Uid

	username := ""
	user, err := user.LookupId(strconv.Itoa(int(uid)))
	if err == nil {
		username = user.Username
	}

	memory_info, _ := MemoryInfoWithContext(ctx, pid)

	result := ordereddict.NewDict().
		SetCaseInsensitive().
		Set("Pid", pid).
		Set("Name", name).
		Set("Ppid", proc.Eproc.Ppid).

		// Make it compatible with the Windows pslist()
		Set("CommandLine", strings.Join(cmdline, " ")).
		Set("_Argv", cmdline).
		Set("CreateTime", time.Unix(proc.Proc.P_starttime.Sec, int64(proc.Proc.P_starttime.Usec)/1000)).
		Set("Times", times).
		Set("Exe", exe).
		Set("Cwd", cwd).
		Set("Uid", uid).
		Set("Username", username).
		Set("MemoryInfo", memory_info)

	return result
}

func IOCountersWithContext(ctx context.Context, pid int32) (*IOCountersStat, error) {
	return nil, NotImplementedError
}

func ByteToString(orig []byte) string {
	n := -1
	l := -1
	for i, b := range orig {
		// skip left side null
		if l == -1 && b == 0 {
			continue
		}
		if l == -1 {
			l = i
		}

		if b == 0 {
			break
		}
		n = i + 1
	}
	if n == -1 {
		return string(orig)
	}
	return string(orig[l:n])
}
