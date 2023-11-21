//go:build windows && amd64
// +build windows,amd64

package process

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vwindows "www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	ThreadBasicInformation          = 0x00
	ThreadQueryInformation          = 0x40
	ThreadQuerySetWin32StartAddress = 0x09
)

type ThreadArgs struct {
	Pid int64 `vfilter:"required,field=pid,doc=The PID to get the thread for."`
}

type ThreadPlugin struct{}

func (self ThreadPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.CheckForPanic(scope, "thread")

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("thread: %s", err)
			return
		}

		arg := &ThreadArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("thread: %s", err.Error())
			return
		}

		err = TryToGrantSeDebugPrivilege()
		if err != nil {
			scope.Log("thread: Cannot get SeDebugPrivilege, %s", err.Error())
			return
		}

		getProcessThreads(scope, arg, output_chan)
	}()

	return output_chan
}

func getProcessThreads(
	scope vfilter.Scope,
	arg *ThreadArgs,
	output_chan chan vfilter.Row) {

	handle, err := windows.CreateToolhelp32Snapshot(
		windows.TH32CS_SNAPTHREAD, uint32(arg.Pid))
	if err != nil {
		scope.Log("CreateToolhelp32Snapshot: %v ", err)
		return
	}
	defer windows.Close(handle)

	entry := windows.ThreadEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Thread32First(handle, &entry)
	if err != nil {
		scope.Log("Thread32First: %v ", err)
		return
	}

	for {
		if entry.OwnerProcessID == uint32(arg.Pid) {
			row, err := checkThread(scope, arg.Pid, entry)
			if err != nil {
				scope.Log("checkThread: %v ", err)
			}

			if row != nil {
				output_chan <- row
			}
		}

		err = windows.Thread32Next(handle, &entry)
		if err != nil {
			break
		}
	}

	return
}

func checkThread(
	scope vfilter.Scope, pid int64,
	entry windows.ThreadEntry32) (vfilter.Row, error) {
	if entry.ThreadID == 0 {
		return nil, errors.New("ThreadID is 0")
	}

	thread, err := windows.OpenThread(ThreadQueryInformation, false, entry.ThreadID)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(thread)

	thread_info := vwindows.THREAD_BASIC_INFORMATION{}
	var length uint32

	status, _ := vwindows.NtQueryInformationThread(
		syscall.Handle(thread),
		vwindows.ThreadBasicInformation,
		(*byte)(unsafe.Pointer(&thread_info)),
		uint32(unsafe.Sizeof(thread_info)),
		&length)
	if status != vwindows.STATUS_SUCCESS || length == 0 {
		return nil, fmt.Errorf("NtQueryInformationProcess failed, %X", status)
	}

	var thread_start_address uint64

	status, _ = vwindows.NtQueryInformationThread(
		syscall.Handle(thread),
		ThreadQuerySetWin32StartAddress,
		(*byte)(unsafe.Pointer(&thread_start_address)),
		uint32(unsafe.Sizeof(thread_start_address)),
		&length)
	if status != vwindows.STATUS_SUCCESS || length == 0 {
		return nil, fmt.Errorf("NtQueryInformationProcess failed, %X", status)
	}

	proc_handle, err := windows.OpenProcess(vwindows.PROCESS_ALL_ACCESS, false, entry.OwnerProcessID)
	if err != nil {
		return nil, err
	}
	defer windows.Close(proc_handle)

	memory_basic_info := &vwindows.MEMORY_BASIC_INFORMATION{}
	mem_length, err := vwindows.VirtualQueryEx(
		syscall.Handle(proc_handle),
		thread_start_address,
		memory_basic_info,
		uintptr(unsafe.Sizeof(*memory_basic_info)))
	if err != nil || mem_length == 0 {
		return nil, err
	}

	// If address space is MEM_IMAGE, get the filename.
	filename := ""
	if memory_basic_info.Type == 0x1000000 {
		wide_filename := make([]uint16, syscall.MAX_PATH)
		len, err := vwindows.GetMappedFileNameW(
			syscall.Handle(proc_handle),
			thread_start_address,
			&wide_filename[0], syscall.MAX_PATH)
		if err == nil {
			filename = syscall.UTF16ToString(wide_filename[:len])
		}
	}

	ret := ordereddict.NewDict().
		Set("pid", pid).
		Set("tid", entry.ThreadID).
		Set("thread_info", thread_info).
		Set("thread_start_address", thread_start_address).
		Set("memory_basic_info", memory_basic_info).
		Set("filename", filename)

	return ret, nil
}

func (self ThreadPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "threads",
		Doc:      "Enumerate threads in a process.",
		ArgType:  type_map.AddType(scope, &ThreadArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ThreadPlugin{})
}
