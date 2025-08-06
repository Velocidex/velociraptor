//go:build linux || darwin
// +build linux darwin

// An accessor for process address space.
// Using this accessor it is possible to read directly from different processes, e.g.
// read_file(filename="/434", accessor="process")

package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/vfilter"
)

const PAGE_SIZE = 0x1000

type ReadAtCloser interface {
	io.ReaderAt
	io.Closer
}

type ProcessReader struct {
	mu     sync.Mutex
	pid    uint64
	offset int64
	size   int64

	// A file handle to the /proc/pid/mem file.
	handle ReadAtCloser
	ranges []*uploads.Range
}

func (self *ProcessReader) Close() error {
	return self.handle.Close()
}

func (self *ProcessReader) Ranges() []uploads.Range {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []uploads.Range{}
	size := int64(0)
	for _, rng := range self.ranges {
		// Fill in a sparse range if needed
		if rng.Offset > size {
			result = append(result, uploads.Range{
				Offset:   size,
				Length:   rng.Offset - size,
				IsSparse: true,
			})
		}

		// Move the pointer past the end of this range.
		size = rng.Offset + rng.Length

		// Add a real data run
		result = append(result, *rng)
	}
	return result
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
		_, err := self.handle.ReadAt(buf[buf_start:buf_end], self.offset)
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

	current_range, next_range := uploads.GetNextRange(self.offset, self.ranges)
	// Current offset is inside the range.
	if current_range != nil {
		to_read := current_range.Offset + current_range.Length - self.offset
		if to_read > int64(len(buf)) {
			to_read = int64(len(buf))
		}

		// Read memory from process at specified offset.
		_, err := self.handle.ReadAt(buf[:to_read], self.offset)

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

	// The current offset is not inside any range so we null pad until
	// the next range.
	if next_range != nil {
		to_read := next_range.Offset - self.offset
		if to_read > int64(len(buf)) {
			to_read = int64(len(buf))
		}

		// Clear the buffer
		for i := range buf[:to_read] {
			buf[i] = 0
		}
		self.offset += to_read
		return int(to_read), nil
	}

	// Range is past the end of file
	return 0, io.EOF
}

func (self *ProcessReader) Seek(offset int64, whence int) (int64, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	switch whence {
	case 0:
		self.offset = offset
	case 1:
		self.offset += offset
	case 2:
		self.offset = self.size
	}

	return int64(self.offset), nil
}

func (self *ProcessReader) Stat() (os.FileInfo, error) {
	full_path, _ := accessors.NewLinuxOSPath(fmt.Sprintf("%v", self.pid))
	return &accessors.VirtualFileInfo{
		Path:  full_path,
		Size_: self.size,
	}, nil
}

type ProcessAccessor struct{}

func (self ProcessAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "process",
		Description: `Access process memory like a file. The Path is taken in the form "/<pid>", i.e. the pid appears as the top level file.`,
		Permissions: []acls.ACL_PERMISSION{acls.MACHINE_STATE},
	}
}

func (self ProcessAccessor) New(scope vfilter.Scope) (accessors.FileSystemAccessor, error) {
	return &ProcessAccessor{}, nil
}

func (self ProcessAccessor) ReadDir(path string) ([]accessors.FileInfo, error) {
	return nil, errors.New("Unable to list all processes, use the pslist() plugin")
}

func (self ProcessAccessor) ReadDirWithOSPath(
	path *accessors.OSPath) ([]accessors.FileInfo, error) {
	return nil, errors.New("Unable to list all processes, use the pslist() plugin")
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

func (self ProcessAccessor) LstatWithOSPath(
	full_path *accessors.OSPath) (accessors.FileInfo, error) {
	return &accessors.VirtualFileInfo{
		Path: full_path,
	}, nil
}

func (self ProcessAccessor) ParsePath(path string) (*accessors.OSPath, error) {
	return accessors.NewLinuxOSPath(path)
}

func (self *ProcessAccessor) Open(
	filename string) (accessors.ReadSeekCloser, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func init() {
	accessors.Register(&ProcessAccessor{})
}
