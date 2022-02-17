// +build windows,amd64,cgo

// An accessor for process address space.
// Using this accessor it is possible to read directly from different processes, e.g.
// read_file(filename="/434", accessor="process")

package process

import (
	"errors"
	"os"
	"strconv"
	"sync"
	"syscall"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/vql/windows"
	"www.velocidex.com/golang/velociraptor/vql/windows/process"
	"www.velocidex.com/golang/vfilter"
)

const PAGE_SIZE = 0x1000

type ProcessReader struct {
	mu         sync.Mutex
	pid        uint64
	offset     uint64
	size       uint64
	handle     syscall.Handle
	ranges     []*process.VMemInfo
	last_range *process.VMemInfo
}

func (self *ProcessReader) getRange(offset uint64) *process.VMemInfo {
	if self.last_range != nil &&
		self.last_range.Address <= offset &&
		offset < self.last_range.Address+self.last_range.Size {
		return self.last_range
	}

	// TODO: Is it worth to implement a binary search here?
	for i := 0; i < len(self.ranges); i++ {
		self.last_range = self.ranges[i]

		// Does the range cover the require offset?
		if self.last_range.Address <= offset &&
			offset < self.last_range.Address+self.last_range.Size {
			return self.last_range
		}

		// Use the fact that ranges are sorted to break early.
		if offset < self.last_range.Address {
			break
		}
	}
	return nil
}

// Repeat the read operation one page at the time in order to retrieve
// as much data as possible.
func (self *ProcessReader) readDistinctPages(buf []byte) (int, error) {
	page_count := len(buf) / PAGE_SIZE
	if page_count <= 1 {
		return page_count * PAGE_SIZE, nil
	}

	// Read as many pages as possible into the buffer ignoring errors.
	for i := 0; i < page_count; i += 1 {
		buf_start := i * PAGE_SIZE
		buf_end := buf_start + PAGE_SIZE

		// Repeat the read with a single page at the time.
		_, err := windows.ReadProcessMemory(
			self.handle, self.offset, buf[buf_start:buf_end])
		if err != nil {
			// Error occured reading a single page, zero
			// it out and skip the page.
			for i := buf_start; i < buf_end; i++ {
				buf[i] = 0
			}
			self.offset += PAGE_SIZE
		}
	}

	return page_count * PAGE_SIZE, nil
}

func (self *ProcessReader) Read(buf []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	current_range := self.getRange(self.offset)
	if current_range == nil {
		return 0, errors.New("Invalid offset")
	}

	to_read := current_range.Address + current_range.Size - self.offset
	if to_read > uint64(len(buf)) {
		to_read = uint64(len(buf))
	}

	// Read memory from process at specified offset.
	_, err := windows.ReadProcessMemory(
		self.handle, self.offset, buf[:to_read])

	// A read error occured - split the read into multiple page
	// size reads to get as much data as we can out of the
	// region. Note: We always return as much data as was
	// required, we simply null pad the missing data. Therefore if
	// a reader askes to read from a memory region that contains
	// no data, we never return an error - just zero pad those
	// regions.
	if err != nil {
		return self.readDistinctPages(buf)
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
		// Only include readable ranges.
		if len(rng.Protection) < 2 || rng.Protection[1] != 'r' {
			continue
		}

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
	return windows.CloseHandle(self.handle)
}

func (self ProcessReader) Stat() (os.FileInfo, error) {
	return &accessors.VirtualFileInfo{Size_: int64(self.size)}, nil
}

type ProcessAccessor struct{}

const _ProcessAccessorTag = "_ProcessAccessor"

func (self ProcessAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	return &ProcessAccessor{}, nil
}

func (self ProcessAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewLinuxOSPath(path)
}

func (self ProcessAccessor) ReadDir(path string) ([]accessors.FileInfo, error) {
	return nil, errors.New("Unable to list all processes, use the pslist() plugin.")
}

func (self ProcessAccessor) ReadDirWithOSPath(
	path *accessors.OSPath) ([]accessors.FileInfo, error) {
	return nil, errors.New("Unable to list all processes, use the pslist() plugin.")
}

func (self ProcessAccessor) Lstat(filename string) (accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return &accessors.VirtualFileInfo{
		Path: full_path,
	}, nil
}

func (self ProcessAccessor) LstatWithOSPath(full_path *accessors.OSPath) (
	accessors.FileInfo, error) {

	return &accessors.VirtualFileInfo{
		Path: full_path,
	}, nil
}

func (self *ProcessAccessor) Open(filename string) (
	accessors.ReadSeekCloser, error) {

	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *ProcessAccessor) OpenWithOSPath(
	full_path *accessors.OSPath) (accessors.ReadSeekCloser, error) {

	if len(full_path.Components) == 0 {
		return nil, errors.New("Unable to list all processes, use the pslist() plugin.")
	}

	pid, err := strconv.ParseUint(full_path.Components[0], 0, 64)
	if err != nil {
		return nil, errors.New("First directory path must be a process.")
	}

	// Open the process and enumerate its ranges
	ranges, proc_handle, err := process.GetVads(uint32(pid))
	if err != nil {
		return nil, err
	}
	result := &ProcessReader{
		pid:    pid,
		handle: proc_handle,
	}

	for _, r := range ranges {
		result.ranges = append(result.ranges, r)
	}

	return result, nil
}

func init() {
	accessors.Register("process", &ProcessAccessor{},
		`Access process memory like a file. The Path is taken in the form "/<pid>", i.e. the pid appears as the top level file.`)
}
