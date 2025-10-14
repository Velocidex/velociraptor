//go:build windows && amd64 && cgo
// +build windows,amd64,cgo

package process

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
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
		defer vql_subsystem.RegisterMonitor(ctx, "thread", args)()

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
	length = 0

	user_times := vwindows.KERNEL_USER_TIMES{}
	status, _ = vwindows.NtQueryInformationThread(
		syscall.Handle(thread),
		vwindows.ThreadTimes,
		(*byte)(unsafe.Pointer(&user_times)),
		uint32(unsafe.Sizeof(user_times)),
		&length)
	if status != vwindows.STATUS_SUCCESS || length == 0 {
		scope.Log("NtQueryInformationProcess failed (ThreadTimes) for pid %v, %X",
			pid, status)
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

	kim := GetKernelInfoManager(scope)
	rva := int64(thread_start_address - memory_basic_info.AllocationBase)
	module_path := kim.NormalizeFilename(filename)
	module_file_name := filepath.Base(module_path)

	func_name := kim.GuessFunctionName(module_path, rva)
	if func_name != "" {
		func_name = fmt.Sprintf("%v!%v", module_file_name, func_name)
	} else {
		func_name = fmt.Sprintf("%v!%#x", module_file_name, rva)
	}

	ret := ordereddict.NewDict().
		Set("pid", pid).
		Set("tid", entry.ThreadID).
		Set("thread_info", thread_info).
		Set("thread_start_address", thread_start_address).
		Set("thread_start_address_name", func_name).
		Set("memory_basic_info", memory_basic_info).
		Set("times", ordereddict.NewDict().
			Set("CreateTime", utils.WinFileTime(int64(user_times.CreateTime))).
			Set("ExitTime", utils.WinFileTime(int64(user_times.ExitTime))).
			Set("KernelTime", user_times.KernelTime).
			Set("UserTime", user_times.UserTime)).
		Set("filename", module_path)

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
