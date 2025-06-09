//go:build darwin && cgo
// +build darwin,cgo

package darwin

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

/*
#include <stdlib.h>
#include <mach/mach_traps.h>
#include <mach/mach.h>
#include <mach/mach_vm.h>
#include <mach/vm_region.h>
#include <mach/vm_statistics.h>
#include <libproc.h>

mach_port_t   get_task_self () ;
*/
import "C"

const (
	// https://opensource.apple.com/source/xnu/xnu-792/osfmk/mach/vm_region.h.auto.html
	VM_REGION_BASIC_INFO          = 10
	VM_REGION_BASIC_INFO_COUNT_64 = 9

	VM_PROT_READ    = 1
	VM_PROT_WRITE   = 2
	VM_PROT_EXECUTE = 4

	MAX_PATH = 1024
)

type VMemInfo struct {
	Address       uint64
	Size          uint64
	MappingName   string
	State         string
	Type          string
	Protection    string
	ProtectionMsg string
	ProtectionRaw uint32
}

type PidArgs struct {
	Pid int64 `vfilter:"required,field=pid,doc=The PID to dump out."`
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
		defer vql_subsystem.CheckForPanic(scope, "vad")
		defer vql_subsystem.RegisterMonitor(ctx, "vad", args)()

		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("vad: %s", err.Error())
			return
		}

		vads, err := GetVads(arg.Pid)
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

func GetVads(pid int64) (res []*VMemInfo, err error) {
	var task C.task_t

	kr := C.task_for_pid(C.get_task_self(), C.int(pid), &task)
	if kr != 0 {
		return nil, fmt.Errorf("process: Can not open pid %v: %v", pid, kr)
	}
	defer func() {
		C.mach_port_deallocate(C.get_task_self(), task)
	}()

	var address C.vm_address_t
	var info_count C.mach_msg_type_number_t = VM_REGION_BASIC_INFO_COUNT_64
	var object C.mach_port_t
	var info C.vm_region_basic_info_data_t
	var size C.vm_size_t

	// Iterate through the address space getting all the regions.
	for {

		kr := C.vm_region_64(task, &address,
			&size, VM_REGION_BASIC_INFO,
			(*C.int)(unsafe.Pointer(&info)), &info_count, &object)
		if kr != 0 {
			break
		}

		// This only seems to work for some mappings.
		buffer := (*C.char)(C.malloc(C.size_t(MAX_PATH)))
		defer C.free(unsafe.Pointer(buffer))

		buf_size := C.uint32_t(MAX_PATH)

		C.proc_regionfilename((C.int)(pid), (C.uint64_t)(address),
			unsafe.Pointer(buffer), buf_size)
		res = append(res, &VMemInfo{
			Address:       uint64(address),
			Size:          uint64(size),
			Protection:    getProtection(uint32(info.protection)),
			ProtectionRaw: uint32(info.protection),
			MappingName:   C.GoString(buffer),
		})
		address += size
		size = 0
	}

	return res, nil
}

func getProtection(p uint32) string {
	result := ""

	if p&VM_PROT_EXECUTE > 0 {
		result += "x"
	} else {
		result += "-"
	}

	if p&VM_PROT_WRITE > 0 {
		result += "w"
	} else {
		result += "-"
	}

	if p&VM_PROT_READ > 0 {
		result += "r"
	} else {
		result += "-"

	}
	return result
}

func init() {
	vql_subsystem.RegisterPlugin(&VADPlugin{})
}
