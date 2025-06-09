//go:build windows
// +build windows

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package windows

import (
	"context"
	"debug/pe"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PslistArgs struct {
	Pid int64 `vfilter:"optional,field=pid,doc=A process ID to list. If not provided list all processes."`
}

type Win32_Process struct {
	Pid             uint32
	Ppid            uint32
	Name            string
	Threads         uint32
	Username        string
	OwnerSid        string
	CommandLine     string
	Exe             string
	TokenIsElevated bool
	CreateTime      time.Time
	User            float64 `json:"user"`
	System          float64 `json:"system"`
	IoCounters      *IO_COUNTERS
	Memory          *PROCESS_MEMORY_COUNTERS
	PebBaseAddress  uint64
	IsWow64         bool
}

type MemoryInfoStat struct {
	RSS    uint64 `json:"rss"`    // bytes
	VMS    uint64 `json:"vms"`    // bytes
	Data   uint64 `json:"data"`   // bytes
	Stack  uint64 `json:"stack"`  // bytes
	Locked uint64 `json:"locked"` // bytes
	Swap   uint64 `json:"swap"`   // bytes
}

func (self *Win32_Process) getHandle() (syscall.Handle, error) {
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, self.Pid)
	if err != nil {
		// Try again to open with limited permissions.
		handle, err = syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION,
			false, self.Pid)
		if err != nil {
			return 0, err
		}
	}

	return handle, nil
}

func (self *Win32_Process) getMemCounters(handle syscall.Handle) {
	var u PROCESS_MEMORY_COUNTERS
	length := uint32(unsafe.Sizeof(u))
	err := GetProcessMemoryInfo(handle, &u, length)
	if err == nil {
		self.Memory = &u
	}
}

func (self *Win32_Process) getIOCounters(handle syscall.Handle) {
	var u IO_COUNTERS

	ok := GetProcessIoCounters(handle, &u)
	if ok {
		self.IoCounters = &u
	}
}

func (self *Win32_Process) getTimes(handle syscall.Handle) {
	var u syscall.Rusage

	err := syscall.GetProcessTimes(handle,
		&u.CreationTime,
		&u.ExitTime,
		&u.KernelTime,
		&u.UserTime)
	if err == nil {
		self.CreateTime = time.Unix(0, u.CreationTime.Nanoseconds())
		self.User = float64(int64(u.UserTime.HighDateTime<<32)+
			int64(u.UserTime.LowDateTime)) / 1e7
		self.System = float64(int64(u.KernelTime.HighDateTime<<32)+
			int64(u.KernelTime.LowDateTime)) / 1e7
	}
}

func (self *Win32_Process) getCmdLine(handle syscall.Handle) {
	buffer := utils.AllocateBuff(1024 * 2)
	length := uint32(0)
	status := NtQueryInformationProcess(handle, ProcessCommandLineInformation,
		(*byte)(unsafe.Pointer(&buffer[0])), uint32(len(buffer)), &length)
	if status == STATUS_SUCCESS {
		self.CommandLine = (*UNICODE_STRING)(unsafe.Pointer(&buffer[0])).String()
	}
}

func (self *Win32_Process) getProcessInfo(handle syscall.Handle) {
	handle_info := PROCESS_BASIC_INFORMATION{}
	var length uint32
	var processMachine, nativeMachine uint16
	err := windows.IsWow64Process2(
		windows.Handle(handle), &processMachine, &nativeMachine)
	if err == nil {
		if processMachine == pe.IMAGE_FILE_MACHINE_I386 {
			self.IsWow64 = true
		}
	}

	status := NtQueryInformationProcess(handle, ProcessBasicInformation,
		(*byte)(unsafe.Pointer(&handle_info)),
		uint32(unsafe.Sizeof(handle_info)), &length)
	if status == STATUS_SUCCESS {
		self.PebBaseAddress = handle_info.PebBaseAddress
	}
}

func (self *Win32_Process) getBinary(handle syscall.Handle) {
	buffer := make([]uint16, 1024)
	length := uint32(len(buffer))
	ok := QueryFullProcessImageName(handle, 0,
		(*byte)(unsafe.Pointer(&buffer[0])), &length)
	if ok {
		self.Exe = syscall.UTF16ToString(buffer[:length])
	}
}

func (self *Win32_Process) getUsername(handle syscall.Handle) {
	var token syscall.Token
	err := syscall.OpenProcessToken(handle, syscall.TOKEN_QUERY, &token)
	if err != nil {
		return
	}
	defer token.Close()

	tokenUser, err := token.GetTokenUser()
	self.OwnerSid, _ = tokenUser.User.Sid.String()

	user, domain, _, err := tokenUser.User.Sid.LookupAccount("")
	self.Username = domain + "\\" + user

	elevation := TOKEN_ELEVATION{}
	length := uint32(0)
	err = syscall.GetTokenInformation(token, syscall.TokenElevation,
		(*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &length)
	if err == nil {
		self.TokenIsElevated = elevation.TokenIsElevated > 0
	}
}

type PslistPlugin struct{}

func (self PslistPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &PslistArgs{}

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "pslist", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("pslist: %v", err)
			return
		}

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		defer vql_subsystem.CheckForPanic(scope, "pslist")

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("pslist: %v", err)
			return
		}

		// If the caller specifies a pid, only query for this
		// pid (0 means all pids).
		handle, err := windows.CreateToolhelp32Snapshot(
			TH32CS_SNAPPROCESS, uint32(arg.Pid))
		if err != nil {
			scope.Log("CreateToolhelp32Snapshot: %v ", err)
			return
		}
		defer windows.Close(handle)

		entry := windows.ProcessEntry32{}
		entry.Size = uint32(unsafe.Sizeof(entry))

		err = windows.Process32First(handle, &entry)
		if err != nil {
			scope.Log("Process32First: %v ", err)
			return
		}

		for {
			if entry.ProcessID != 0 &&
				(arg.Pid == 0 || arg.Pid == int64(entry.ProcessID)) {
				info := &Win32_Process{
					Pid:     entry.ProcessID,
					Ppid:    entry.ParentProcessID,
					Name:    syscall.UTF16ToString(entry.ExeFile[:]),
					Threads: entry.Threads,
				}

				proc_handle, err := info.getHandle()
				if err == nil {
					info.getCmdLine(proc_handle)
					info.getProcessInfo(proc_handle)
					info.getBinary(proc_handle)
					info.getUsername(proc_handle)
					info.getTimes(proc_handle)
					info.getIOCounters(proc_handle)
					info.getMemCounters(proc_handle)

					// Close the handle now.
					syscall.Close(proc_handle)
				}
				select {
				case <-ctx.Done():
					return
				case output_chan <- info:
				}
			}
			err = windows.Process32Next(handle, &entry)
			if err == syscall.ERROR_NO_MORE_FILES {
				return
			} else if err != nil {
				scope.Log("Process32Next: %v ", err)
				return
			}
		}

	}()

	return output_chan
}

func (self PslistPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "pslist",
		Doc:      "Enumerate running processes.",
		ArgType:  type_map.AddType(scope, &PslistArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&PslistPlugin{})
}
