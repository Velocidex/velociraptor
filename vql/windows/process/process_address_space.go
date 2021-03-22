// +build windows,amd64

// An accessor for process address space.
// Using this accessor it is possible to read directly from different processes, e.g.
// read_file(filename="/434", accessor="process")

package process

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"syscall"

	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/vfilter"
)

type ProcessFileInfo struct {
	utils.DataFileInfo
	size uint64
}

func (self ProcessFileInfo) Size() int64 {
	return int64(self.size)
}

type ProcessReader struct {
	mu     sync.Mutex
	pid    int
	offset uint64
	size   uint64

	handle     syscall.Handle
	ranges     []*VMemeInfo
	last_range *VMemeInfo
}

func (self *ProcessReader) getRange(offset uint64) *VMemeInfo {
	if self.last_range != nil &&
		self.last_range.Address <= offset &&
		offset < self.last_range.Address+self.last_range.Size {
		return self.last_range
	}

	for i := 0; i < len(self.ranges); i++ {
		self.last_range = self.ranges[i]

		if self.last_range.Address <= offset &&
			offset < self.last_range.Address+self.last_range.Size {
			return self.last_range
		}
	}

	return nil
}

func (self *ProcessReader) Read(buf []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	fmt.Printf("Reading %#0x - %#0x\n", self.offset, len(buf))

	current_range := self.getRange(self.offset)
	if current_range == nil {
		return 0, errors.New("Invalid offset")
	}

	to_read := current_range.Address + current_range.Size - self.offset
	if to_read > uint64(len(buf)) {
		to_read = uint64(len(buf))
	}

	// Read memory from process at specified offset.
	n, _ := windows.ReadProcessMemory(
		self.handle, self.offset, buf[:to_read])

	// Reading the process produced less data than required, we
	// therefore zero pad it and return the full buffer.
	len_read := uint64(n)
	if len_read < to_read {
		for i := len_read; i < to_read; i++ {
			buf[i] = 0
		}
	}

	// Advance the read pointer.
	self.offset += to_read

	return int(to_read), nil
}

func (self *ProcessReader) Ranges() []uploads.Range {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []uploads.Range{}
	size := uint64(0)
	for _, rng := range self.ranges {
		// Fill in a sparse range if needed
		if rng.Address > size {
			result = append(result, uploads.Range{
				Offset:   int64(size),
				Length:   int64(rng.Address - size),
				IsSparse: true,
			})
		}

		// Move the pointer past the end of this range.
		size = rng.Address + rng.Size

		// Add a real data run
		result = append(result, uploads.Range{
			Offset:   int64(rng.Address),
			Length:   int64(rng.Size),
			IsSparse: false,
		})
	}
	return result
}

func (self *ProcessReader) Seek(offset int64, whence int) (int64, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	switch whence {
	case 0:
		self.offset = uint64(offset)
	case 1:
		self.offset += uint64(offset)
	case 2:
		self.offset = self.size
	}

	return int64(self.offset), nil
}

func (self ProcessReader) Close() error {
	fmt.Printf("Closing handle\n")
	return windows.CloseHandle(self.handle)
}

func (self ProcessReader) Stat() (os.FileInfo, error) {
	return &ProcessFileInfo{size: self.size}, nil
}

type ProcessAccessor struct {
	glob.DataFilesystemAccessor

	mu    sync.Mutex
	cache map[uint32]*ProcessReader
}

const _ProcessAccessorTag = "_ProcessAccessor"

func (self ProcessAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	result_any := vql_subsystem.CacheGet(scope, _ProcessAccessorTag)
	if result_any == nil {
		result := &ProcessAccessor{
			cache: make(map[uint32]*ProcessReader),
		}
		vql_subsystem.CacheSet(scope, _ProcessAccessorTag, result)
		vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			for _, v := range result.cache {
				v.Close()
			}
		})

		return result, nil
	}

	return result_any.(glob.FileSystemAccessor), nil
}

func (self ProcessAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	return nil, errors.New("Unable to list all processes, use the pslist() plugin.")
}

func (self ProcessAccessor) Lstat(filename string) (glob.FileInfo, error) {
	return utils.NewDataFileInfo("hello"), nil
}

func (self *ProcessAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	components := utils.SplitComponents(path)
	if len(components) == 0 {
		return nil, errors.New("Unable to list all processes, use the pslist() plugin.")
	}

	pid, err := strconv.Atoi(components[0])
	if err != nil {
		return nil, errors.New("First directory path must be a process.")
	}

	result, pres := self.cache[uint32(pid)]
	if pres {
		return result, nil
	}

	// Open the process and enumerate its ranges
	ranges, proc_handle, err := GetVads(uint32(pid))
	if err != nil {
		return nil, err
	}
	fmt.Printf("Open %v with handle %v\n", pid, proc_handle)
	result = &ProcessReader{
		pid:    pid,
		handle: proc_handle,
	}

	for _, r := range ranges {
		// Only include readable ranges.
		if len(r.Protection) < 2 || r.Protection[1] != 'r' {
			continue
		}
		result.ranges = append(result.ranges, r)
	}

	//self.cache[uint32(pid)] = result

	return result, nil
}

func init() {
	glob.Register("process", &ProcessAccessor{})
}
