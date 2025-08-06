//go:build windows && amd64 && cgo
// +build windows,amd64,cgo

package winpmem

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Velocidex/WinPmem/go-winpmem"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// An accessor for physical memory. Uses the winpmem driver to gain
// access to the physical memory.

const (
	PAGE_SIZE  = 0x1000
	DeviceName = `\\.\pmem`
)

type WinpmemReader struct {
	*winpmem.Imager

	mu     sync.Mutex
	offset int64
}

func (self *WinpmemReader) Read(buf []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	n, err := self.ReadAt(buf, self.offset)
	self.offset += int64(n)

	return n, err
}

func (self *WinpmemReader) Ranges() []uploads.Range {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []uploads.Range{}
	size := int64(0)
	for _, rng := range self.Stats().Run {
		// Fill in a sparse range if needed
		if int64(rng.BaseAddress) > size {
			result = append(result, uploads.Range{
				Offset:   int64(size),
				Length:   int64(rng.BaseAddress) - size,
				IsSparse: true,
			})
		}

		// Move the pointer past the end of this range.
		size = int64(rng.BaseAddress + rng.NumberOfBytes)

		// Add a real data run
		result = append(result, uploads.Range{
			Offset:   int64(rng.BaseAddress),
			Length:   int64(rng.NumberOfBytes),
			IsSparse: false,
		})
	}
	return result
}

func (self *WinpmemReader) Seek(offset int64, whence int) (int64, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	switch whence {
	case 0:
		self.offset = offset
	case 1:
		self.offset += offset
	case 2:
		return 0, utils.NotImplementedError
	}

	return int64(self.offset), nil
}

// Keep the process alive in cache for a bit
func (self *WinpmemReader) Close() error {
	return nil
}

func (self WinpmemReader) Stat() (os.FileInfo, error) {
	return &accessors.VirtualFileInfo{}, nil
}

type WinpmemAccessor struct {
	scope  vfilter.Scope
	imager *winpmem.Imager
}

const _WinpmemAccessorTag = "_WinpmemAccessor"

func (self WinpmemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {

	result_any := vql_subsystem.CacheGet(scope, _WinpmemAccessorTag)
	if result_any == nil {
		logger := NewLogger(scope, "winpmem accessor: ")
		imager, err := winpmem.NewImager(DeviceName, logger)
		if err != nil {
			return nil, fmt.Errorf("winpmem: unable to load device, ensure to load it with the winpmem() function first: %w", err)
		}

		// We only support this mode now - it is the most reliable.
		imager.SetMode(winpmem.PMEM_MODE_PTE)

		// Create a new cache in the scope.
		result := &WinpmemAccessor{
			scope:  scope,
			imager: imager,
		}
		vql_subsystem.CacheSet(scope, _WinpmemAccessorTag, result)

		vql_subsystem.GetRootScope(scope).AddDestructor(func() {
			imager.Close()
		})

		return result, nil
	}

	return result_any.(*WinpmemAccessor), nil
}

func (self WinpmemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "winpmem",
		Description: `Access physical memory like a file. Any filename will result in a sparse view of physical memory.`,
		Permissions: []acls.ACL_PERMISSION{acls.MACHINE_STATE},
	}
}

func (self WinpmemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewLinuxOSPath(path)
}

func (self WinpmemAccessor) ReadDir(path string) ([]accessors.FileInfo, error) {
	return nil, errors.New("winpmem accessor: Directory operations not supported.")
}

func (self WinpmemAccessor) ReadDirWithOSPath(
	path *accessors.OSPath) ([]accessors.FileInfo, error) {
	return nil, errors.New("winpmem accessor: Directory operations not supported.")
}

func (self WinpmemAccessor) Lstat(filename string) (accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return &accessors.VirtualFileInfo{
		Path: full_path,
	}, nil
}

func (self WinpmemAccessor) LstatWithOSPath(full_path *accessors.OSPath) (
	accessors.FileInfo, error) {

	return &accessors.VirtualFileInfo{
		Path: full_path,
	}, nil
}

func (self *WinpmemAccessor) Open(filename string) (
	accessors.ReadSeekCloser, error) {

	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

// Open the same imager for all paths
func (self *WinpmemAccessor) OpenWithOSPath(
	full_path *accessors.OSPath) (accessors.ReadSeekCloser, error) {

	return &WinpmemReader{
		Imager: self.imager,
	}, nil
}

func init() {
	accessors.Register(&WinpmemAccessor{})
}
