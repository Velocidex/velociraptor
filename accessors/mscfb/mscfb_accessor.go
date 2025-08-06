package mscfb

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/Velocidex/go-mscfb/parser"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/vfilter"
)

type MscfbFileInfo struct {
	entry      *parser.DirectoryEntry
	_full_path *accessors.OSPath
}

func (self *MscfbFileInfo) Name() string {
	return self.entry.Name
}

func (self *MscfbFileInfo) UniqueName() string {
	return self._full_path.String()
}

func (self *MscfbFileInfo) IsDir() bool {
	return self.entry.IsDir
}

func (self *MscfbFileInfo) Data() *ordereddict.Dict {
	return ordereddict.NewDict()
}

func (self *MscfbFileInfo) FullPath() string {
	return self._full_path.String()
}

func (self *MscfbFileInfo) OSPath() *accessors.OSPath {
	return self._full_path
}

// Not supported
func (self *MscfbFileInfo) IsLink() bool {
	return false
}

func (self *MscfbFileInfo) GetLink() (*accessors.OSPath, error) {
	return nil, errors.New("Not implemented")
}

func (self *MscfbFileInfo) Mtime() time.Time {
	return self.entry.Mtime
}

func (self *MscfbFileInfo) ModTime() time.Time {
	return self.entry.Mtime
}

func (self *MscfbFileInfo) Atime() time.Time {
	return time.Time{}
}

func (self *MscfbFileInfo) Ctime() time.Time {
	return self.entry.Ctime
}

func (self *MscfbFileInfo) Btime() time.Time {
	return self.entry.Ctime
}

func (self *MscfbFileInfo) Size() int64 {
	return int64(self.entry.Size)
}

func (self *MscfbFileInfo) Mode() os.FileMode {
	var result os.FileMode = 0755
	if self.IsDir() {
		result |= os.ModeDir
	}
	return result
}

type MscfbFileSystemAccessor struct {
	scope vfilter.Scope

	// The delegate accessor we use to open the underlying volume.
	accessor string
	device   *accessors.OSPath

	root *accessors.OSPath
}

func (self MscfbFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "mscfb",
		Description: `Parse a MSCFB file as an archive.`,
	}
}

func (self MscfbFileSystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	// Create a new cache in the scope.
	return &MscfbFileSystemAccessor{
		scope:    scope,
		device:   self.device,
		accessor: self.accessor,
		root:     self.root,
	}, nil
}

func (self MscfbFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewLinuxOSPath(path)
}

func (self *MscfbFileSystemAccessor) ReadDir(path string) (
	res []accessors.FileInfo, err error) {
	// Normalize the path
	fullpath, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(fullpath)
}

func (self *MscfbFileSystemAccessor) ReadDirWithOSPath(
	fullpath *accessors.OSPath) (res []accessors.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	result := []accessors.FileInfo{}
	if len(fullpath.Components) > 0 {
		return nil, errors.New("Not found error")
	}

	ole_ctx, err := GetMscfbContext(
		self.scope, self.device, fullpath, self.accessor)
	if err != nil {
		return nil, err
	}

	// List the directory.
	for _, info := range ole_ctx.Directories {
		result = append(result, &MscfbFileInfo{
			entry:      &info,
			_full_path: fullpath.Append(info.Name),
		})
	}
	return result, nil
}

func (self *MscfbFileSystemAccessor) Open(
	path string) (res accessors.ReadSeekCloser, err error) {

	full_path, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self *MscfbFileSystemAccessor) OpenWithOSPath(
	fullpath *accessors.OSPath) (res accessors.ReadSeekCloser, err error) {

	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	if len(fullpath.Components) != 1 {
		return nil, errors.New("Not found error")
	}

	ole_ctx, err := GetMscfbContext(
		self.scope, self.device, fullpath, self.accessor)
	if err != nil {
		return nil, err
	}

	// Open the device path from the root.
	stream, dir, err := ole_ctx.Open(fullpath.Components[0])
	if err != nil {
		return nil, err
	}

	return &readAdapter{
		info: &MscfbFileInfo{
			entry:      dir,
			_full_path: fullpath,
		},
		reader: stream,
	}, nil
}

func (self *MscfbFileSystemAccessor) Lstat(
	path string) (res accessors.FileInfo, err error) {

	fullpath, err := self.ParsePath(path)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(fullpath)
}

func (self *MscfbFileSystemAccessor) LstatWithOSPath(
	fullpath *accessors.OSPath) (res accessors.FileInfo, err error) {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Printf("PANIC %v\n", r)
			debug.PrintStack()
			err, _ = r.(error)
		}
	}()

	ole_ctx, err := GetMscfbContext(
		self.scope, self.device, fullpath, self.accessor)
	if err != nil {
		return nil, err
	}

	var dir *parser.DirectoryEntry
	if len(fullpath.Components) > 1 {
		return nil, errors.New("Not found error")
	}

	if len(fullpath.Components) == 0 {
		// Root directory
		dir, err = ole_ctx.GetDirentry(0)
	} else {

		dir, err = ole_ctx.Stat(fullpath.Components[0])
	}

	return &MscfbFileInfo{
		entry:      dir,
		_full_path: fullpath,
	}, err
}

func init() {
	accessors.Register(&MscfbFileSystemAccessor{})

	json.RegisterCustomEncoder(&MscfbFileInfo{}, accessors.MarshalGlobFileInfo)
}
