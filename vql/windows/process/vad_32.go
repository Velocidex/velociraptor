// +build windows,386,cgo

package process

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
)

type VMemeInfo struct {
	Address     uint64
	Size        uint64
	MappingName string
	Protection  string
}

type ModuleInfo struct {
	ProcessID         uint32
	ModuleBaseAddress uint32
	ModuleBaseSize    uint32
	ModuleName        string
	ExePath           string
}

type ModulesPlugin struct{}

func (self ModulesPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &PidArgs{}

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("modules: %s", err)
			return
		}

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		defer vql_subsystem.CheckForPanic(scope, "module")

		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("modules: %s", err.Error())
			return
		}

		modules, err := GetProcessModules(uint32(arg.Pid))
		if err != nil {
			scope.Log("modules: %s", err.Error())
			return
		}

		for _, mod := range modules {
			select {
			case <-ctx.Done():
				return
			case output_chan <- mod:
			}
		}

	}()

	return output_chan
}

func (self ModulesPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "modules",
		Doc:     "Enumerate Loaded DLLs.",
		ArgType: type_map.AddType(scope, &PidArgs{}),
	}
}

type VADPlugin struct{}

func (self VADPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	arg := &PidArgs{}

	go func() {
		defer close(output_chan)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		defer vql_subsystem.CheckForPanic(scope, "module")

		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("vad: %s", err.Error())
			return
		}

		vads, err := GetVads(uint32(arg.Pid))
		if err != nil {
			scope.Log("vad: %s", err.Error())
			return
		}

		for _, vad := range vads {
			select {
			case <-ctx.Done():
				return
			case output_chan <- vad:
			}
		}
	}()

	return output_chan
}

func (self VADPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "vad",
		Doc:     "Enumerate process memory regions.",
		ArgType: type_map.AddType(scope, &PidArgs{}),
	}
}

func GetVads(pid uint32) ([]*VMemeInfo, error) {
	result := []*VMemeInfo{}

	proc_handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ,
		false, pid)
	if err != nil {
		return nil, errors.New(
			fmt.Sprintf("OpenProcess for pid %v: %v ", pid, err))
	}

	var si windows.SYSTEM_INFO
	windows.GetSystemInfo(&si)

	min_application_address := si.MinimumApplicationAddress
	max_application_address := si.MaximumApplicationAddress

	for i := min_application_address; i < max_application_address; {
		info := windows.MEMORY_BASIC_INFORMATION{}
		_, err := windows.VirtualQueryEx(proc_handle, i, &info,
			unsafe.Sizeof(info))

		if err != nil {
			return result, err
		}

		filename := ""
		wide_filename := make([]uint16, syscall.MAX_PATH)
		len, err := windows.GetMappedFileNameW(proc_handle, i,
			&wide_filename[0], syscall.MAX_PATH)
		if err == nil {
			filename = syscall.UTF16ToString(wide_filename[:len])
		}

		// Ignore pages with no access.
		if info.Protect != windows.PAGE_NOACCESS {
			result = append(result, &VMemeInfo{
				Address:     info.BaseAddress,
				Size:        info.RegionSize,
				MappingName: filename,
				Protection:  getProtection(info.Protect),
			})
		}

		if info.RegionSize == 0 {
			break
		}

		i += info.RegionSize
	}

	return result, nil
}

func GetProcessModules(pid uint32) ([]ModuleInfo, error) {
	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPMODULE | windows.TH32CS_SNAPMODULE32, pid)
	if err != nil {
		return nil, errors.New(
			fmt.Sprintf("CreateToolhelp32Snapshot for pid %v: %v ", pid, err))
	}
	defer windows.CloseHandle(handle)

	mod_entry := windows.MODULEENTRY32W{}
	mod_entry.Size = uint32(unsafe.Sizeof(mod_entry))

	err = windows.Module32First(handle, &mod_entry)
	if err != nil {
		return nil, errors.New(
			fmt.Sprintf("Module32First for pid %v: %v ", pid, err))
	}

	mi := []ModuleInfo{}
	for {
		mi = append(mi, ModuleInfo{
			ProcessID:         mod_entry.ProcessID,
			ModuleBaseAddress: mod_entry.ModBaseAddr,
			ModuleBaseSize:    mod_entry.ModBaseSize,
			ModuleName:        syscall.UTF16ToString((&mod_entry.ModuleName)[:]),
			ExePath:           syscall.UTF16ToString((&mod_entry.ExePath)[:]),
		})
		err := windows.Module32Next(handle, &mod_entry)
		if err == syscall.ERROR_NO_MORE_FILES {
			return mi, nil
		} else if err != nil {
			return nil, errors.New(
				fmt.Sprintf("Module32Next for pid %v: %v ", pid, err))
		}
	}
}

func getProtection(p uint32) string {
	result := []string{}
	if p&windows.PAGE_EXECUTE > 0 {
		result = append(result, "x--")
	}
	if p&windows.PAGE_EXECUTE_READ > 0 {
		result = append(result, "xr-")
	}
	if p&windows.PAGE_EXECUTE_READWRITE > 0 {
		result = append(result, "xrw")
	}
	if p&windows.PAGE_EXECUTE_WRITECOPY > 0 {
		result = append(result, "x-w")
	}
	if p&windows.PAGE_NOACCESS > 0 {
		result = append(result, "---")
	}
	if p&windows.PAGE_READONLY > 0 {
		result = append(result, "-r-")
	}
	if p&windows.PAGE_READWRITE > 0 {
		result = append(result, "-rw")
	}
	if p&windows.PAGE_WRITECOPY > 0 {
		result = append(result, "--w")
	}

	return strings.Join(result, ",")
}

func init() {
	vql_subsystem.RegisterPlugin(&VADPlugin{})
	vql_subsystem.RegisterPlugin(&ModulesPlugin{})
}
