// +build windows

// References: https://www.geoffchappell.com/studies/windows/km/ntoskrnl/api/ex/sysinfo/query.htm
// https://processhacker.sourceforge.io/

package process

import (
	"context"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
)

type HandleInfo struct {
	Pid    uint32
	Type   string
	Name   string
	Handle uint32
}

type HandlesPluginArgs struct {
	Pid   uint64   `vfilter:"optional,field=pid,doc=If specified only get handles from these PIDs."`
	Types []string `vfilter:"optional,field=types,doc=If specified only get handles of this type."`
}

type HandlesPlugin struct{}

func (self HandlesPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		runtime.LockOSThread()

		// Deliberately do not unlock this thread - this will
		// cause Go to terminate it and start another one.
		// defer runtime.UnlockOSThread()

		defer vql_subsystem.CheckForPanic(scope, "handles")

		arg := &HandlesPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("handles: %s", err.Error())
			return
		}

		err = TryToGrantSeDebugPrivilege()
		if err != nil {
			scope.Log("handles while trying to grant SeDebugPrivilege: %s", err.Error())
		}

		GetHandles(scope, arg, output_chan)
	}()

	return output_chan
}

func (self HandlesPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "handles",
		Doc:     "Enumerate process handles.",
		ArgType: type_map.AddType(scope, &ProcDumpArgs{}),
	}
}

func is_type_chosen(types []string, objtype string) bool {
	if len(types) == 0 {
		return true
	}

	for _, i := range types {
		if i == objtype {
			return true
		}
	}

	return false
}

func GetHandles(scope *vfilter.Scope, arg *HandlesPluginArgs, out chan<- vfilter.Row) {
	// This should be large enough to fit all the handles.
	buffer := make([]byte, 1024*1024*4)

	// Group all handles by pid
	pid_map := make(map[int][]*windows.SYSTEM_HANDLE_TABLE_ENTRY_INFO64)

	var length uint32

	status := windows.NtQuerySystemInformation(windows.SystemHandleInformation,
		&buffer[0], uint32(len(buffer)), &length)
	if status != windows.STATUS_SUCCESS {
		scope.Log("NtQuerySystemInformation status " +
			windows.NTStatus_String(status))
		return
	}

	// First pass, group all handles by pid.
	size := int(unsafe.Sizeof(windows.SYSTEM_HANDLE_TABLE_ENTRY_INFO64{}))
	for i := 8; i < int(length); i += size {
		handle_info := (*windows.SYSTEM_HANDLE_TABLE_ENTRY_INFO64)(unsafe.Pointer(
			uintptr(unsafe.Pointer(&buffer[0])) + uintptr(i)))

		pid := int(handle_info.UniqueProcessId)
		handle_group, _ := pid_map[pid]
		handle_group = append(handle_group, handle_info)
		pid_map[pid] = handle_group
	}

	// Now for each pid, inspect the handles carefully.
	for pid, handle_group := range pid_map {
		if arg.Pid != 0 && arg.Pid != uint64(pid) {
			continue
		}

		func() {
			process_handle := windows.NtCurrentProcess()
			my_pid := os.Getpid()

			// Open a handle to this process.
			if pid != my_pid {
				h, err := windows.OpenProcess(
					windows.PROCESS_ALL_ACCESS|
						windows.PROCESS_DUP_HANDLE,
					true, uint32(pid))
				if err != nil {
					scope.Log("OpenProcess for pid %v: %v\n", pid, err)
					return
				}
				process_handle = h
				defer windows.CloseHandle(h)
			}

			// Duplicate each handle and query its details.
			for _, handle_info := range handle_group {
				handle_value := syscall.Handle(handle_info.HandleValue)

				// If we do not own the handle we need
				// to dup it into our process. If the
				// handle is already in our process we
				// can use it as is.
				if int(handle_info.UniqueProcessId) != my_pid {
					dup_handle := syscall.Handle(0)
					status := windows.NtDuplicateObject(
						process_handle, handle_value,
						windows.NtCurrentProcess(),
						&dup_handle,
						windows.PROCESS_QUERY_LIMITED_INFORMATION, 0, 0)
					if status == windows.STATUS_SUCCESS {
						SendHandleInfo(
							arg, scope,
							handle_info,
							dup_handle, out)
						windows.CloseHandle(dup_handle)
					}
				} else {
					SendHandleInfo(
						arg, scope, handle_info,
						handle_value, out)
				}
			}
		}()
	}
}

func SendHandleInfo(arg *HandlesPluginArgs, scope *vfilter.Scope,
	handle_info *windows.SYSTEM_HANDLE_TABLE_ENTRY_INFO64,
	handle syscall.Handle, out chan<- vfilter.Row) {

	to_send := false
	result := &HandleInfo{
		Pid:    uint32(handle_info.UniqueProcessId),
		Handle: uint32(handle_info.HandleValue),
	}

	// Sometimes the NtQueryObject blocks without a
	// reason. Process Hacker uses a strategy where it launches
	// the call on another thread and actively kills the
	// thread. Instead we just sacrifice an Go thread. This may
	// not be ideal.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	go func() {
		defer cancel()

		result.Type = GetObjectType(handle, scope)
		if is_type_chosen(arg.Types, result.Type) {
			to_send = true
			result.Name = GetObjectName(handle, scope)
		}
	}()

	select {
	case <-ctx.Done():
		break
	}

	if to_send {
		out <- result
	}
}

func GetObjectName(handle syscall.Handle, scope *vfilter.Scope) string {
	buffer := make([]byte, 1024*2)

	var length uint32

	status, _ := windows.NtQueryObject(handle, windows.ObjectNameInformation,
		&buffer[0], uint32(len(buffer)), &length)

	if status == windows.STATUS_SUCCESS {
		return (*windows.UNICODE_STRING)(unsafe.Pointer(&buffer[0])).String()
	}
	scope.Log("GetObjectName status %v", windows.NTStatus_String(status))
	return ""
}

func GetObjectType(handle syscall.Handle, scope *vfilter.Scope) string {
	buffer := make([]byte, 1024*10)
	length := uint32(0)
	status, _ := windows.NtQueryObject(handle, windows.ObjectTypeInformation,
		&buffer[0], uint32(len(buffer)), &length)

	if status == windows.STATUS_SUCCESS {
		return (*windows.OBJECT_TYPE_INFORMATION)(
			unsafe.Pointer(&buffer[0])).TypeName.String()
	}
	scope.Log("GetObjectType status %v", windows.NTStatus_String(status))
	return ""
}

// Useful for access permissions.
func GetObjectBasicInformation(handle syscall.Handle) *windows.OBJECT_BASIC_INFORMATION {
	result := windows.OBJECT_BASIC_INFORMATION{}
	length := uint32(0)
	windows.NtQueryObject(handle, windows.ObjectBasicInformation,
		(*byte)(unsafe.Pointer(&result)), uint32(unsafe.Sizeof(result)), &length)

	return &result
}

func init() {
	vql_subsystem.RegisterPlugin(&HandlesPlugin{})
}
