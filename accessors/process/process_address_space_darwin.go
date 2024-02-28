//go:build darwin && cgo
// +build darwin,cgo

// An accessor for process address space.
// Using this accessor it is possible to read directly from different processes, e.g.
// read_file(filename="/434", accessor="process")

package process

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"unsafe"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/uploads"
)

/*

#include <mach/mach_traps.h>
#include <mach/mach.h>
#include <mach/mach_vm.h>
#include <mach/vm_region.h>
#include <mach/vm_statistics.h>

mach_port_t   get_task_self () {
 return mach_task_self();
}

// Override go type checking - xnu converts void * to vm_address_t (ulong)
// which is unsafe on 32 bit platforms.
kern_return_t
_vm_read_overwrite(
	vm_map_t        map,
	vm_address_t    address,
	vm_size_t       size,
	char            *data,
	vm_size_t       *data_size) {
  return vm_read_overwrite(map, address, size, (vm_address_t)(data), data_size);
};

*/
import "C"

const (
	// https://opensource.apple.com/source/xnu/xnu-792/osfmk/mach/vm_region.h.auto.html
	VM_REGION_BASIC_INFO          = 10
	VM_REGION_BASIC_INFO_COUNT_64 = 9
)

type darwinProcessReader struct {
	task C.task_t
}

func (self darwinProcessReader) ReadAt(buff []byte, offset int64) (int, error) {
	var size C.ulong

	kr := C._vm_read_overwrite(self.task, (C.vm_address_t)(offset),
		C.ulong(len(buff)), (*C.char)(unsafe.Pointer(&buff[0])),
		&size)
	if kr != 0 {
		return int(size), io.EOF
	}

	return int(size), nil
}

func (self darwinProcessReader) Close() error {
	C.mach_port_deallocate(C.get_task_self(), self.task)
	return nil
}

func (self *ProcessAccessor) OpenWithOSPath(
	path *accessors.OSPath) (accessors.ReadSeekCloser, error) {
	if len(path.Components) == 0 {
		return nil, errors.New("Unable to list all processes, use the pslist() plugin.")
	}

	pid, err := strconv.ParseUint(path.Components[0], 0, 64)
	if err != nil {
		return nil, errors.New("First directory path must be a process.")
	}

	reader := &darwinProcessReader{}

	kr := C.task_for_pid(C.get_task_self(), C.int(pid), &reader.task)
	if kr != 0 {
		return nil, fmt.Errorf("process: Can not open pid %v: %v", pid, kr)
	}

	ranges, err := GetVads(pid, reader.task)
	if err != nil {
		return nil, err
	}

	result := &ProcessReader{
		pid:    pid,
		ranges: ranges,
		handle: reader,
	}
	return result, nil
}

func GetVads(pid uint64, task C.task_t) (res []*uploads.Range, err error) {
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

		res = append(res, &uploads.Range{
			Offset: int64(address),
			Length: int64(size),
		})
		address += size
		size = 0
	}
	return res, nil
}
