// +build windows

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
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
	buffer := make([]byte, 1024*2)
	length := uint32(0)
	status := NtQueryInformationProcess(handle, ProcessCommandLineInformation,
		(*byte)(unsafe.Pointer(&buffer[0])), uint32(len(buffer)), &length)
	if status == STATUS_SUCCESS {
		self.CommandLine = (*UNICODE_STRING)(unsafe.Pointer(&buffer[0])).String()
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
	if err != nil {
		return
	}

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
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &PslistArgs{}

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("pslist: %s", err)
			return
		}

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		defer vql_subsystem.CheckForPanic(scope, "pslist")

		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("pslist: %s", err.Error())
			return
		}

		handle, err := windows.CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
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
					info.getBinary(proc_handle)
					info.getUsername(proc_handle)
					info.getTimes(proc_handle)
					info.getIOCounters(proc_handle)
					info.getMemCounters(proc_handle)

					// Close the handle now.
					syscall.Close(proc_handle)
				}

				output_chan <- info
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

func (self PslistPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "pslist",
		Doc:     "Enumerate running processes.",
		ArgType: type_map.AddType(scope, &PslistArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&PslistPlugin{})
}
