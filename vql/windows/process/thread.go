//go:build windows && amd64
// +build windows,amd64

package process

import (
	"context"
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
	THREAD_BASIC_INFORMATION = 0x0008
)

type ThreadArgs struct {
	Pid             int64  `vfilter:"required,field=pid,doc=The PID to get the thread for."`
	ThreadInfoClass string `vfilter:"optional,field=thread_info_class,doc=The thread information class to query."`
}

type ThreadFunction struct{}

func (self ThreadFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)

	if err != nil {
		scope.Log("thread: %s", err)
		return vfilter.Null{}
	}

	arg := &ThreadArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("thread: %s", err.Error())
		return vfilter.Null{}
	}

	TryToGrantSeDebugPrivilege()

	handle, err := windows.CreateToolhelp32Snapshot(
		windows.TH32CS_SNAPTHREAD, uint32(arg.Pid))
	if err != nil {
		scope.Log("CreateToolhelp32Snapshot: %v ", err)
		return vfilter.Null{}
	}
	defer windows.Close(handle)

	entry := windows.ThreadEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Thread32First(handle, &entry)
	if err != nil {
		scope.Log("Thread32First: %v ", err)
		return vfilter.Null{}
	}

	for {
		if entry.OwnerProcessID != uint32(arg.Pid) {
			thread, err := windows.OpenThread(THREAD_BASIC_INFORMATION, false, entry.ThreadID)
			if err != nil {
				continue
			}

			var info vwindows.THREAD_BASIC_INFORMATION
			buffer := make([]byte, unsafe.Sizeof(info))
			var return_length uint32

			vwindows.NtQueryInformationThread(
				syscall.Handle(thread),
				vwindows.ThreadBasicInformation,
				&buffer[0],
				uint32(unsafe.Sizeof(info)),
				&return_length)

		}

		err = windows.Thread32Next(handle, &entry)
		if err != nil {
			break
		}
	}
}
