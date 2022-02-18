package accessors

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// An OS Path can be thought of as a sequence of components. The OS
// APIs receive a single string, which is the serialization of the OS
// Path. Different operating systems have different serialization
// methods to arrive at the same OS Path.

// For example, on Windows components are separated by the backslash
// characted and the first component can be a device name (which may
// contain path separators):

// \\.\C:\Windows\System32 -> ["\\.\C:", "Windows", "System32"]
// C:\Windows\System32 -> ["C:", "Windows", "System32"]

// On Linux, the path separator is / and serializations start with "/"

// /usr/bin/ls -> ["usr", "bin", "ls"]

// In Velociraptor we try to keep the OS Path object intact as far as
// possible and only serialize to OS representation when necessary.

type PathManipulator interface {
	PathParse(path string, result *OSPath) error
	PathJoin(path *OSPath) string
	AsPathSpec(path *OSPath) *PathSpec
}

type OSPath struct {
	Components []string

	// Some paths need more information. They store an additional path
	// spec here.
	pathspec    *PathSpec
	serialized  *string
	Manipulator PathManipulator
}

// Make a copy of the OSPath
func (self OSPath) Copy() *OSPath {
	pathspec := self.pathspec
	if pathspec != nil {
		pathspec = pathspec.Copy()
	}
	return &OSPath{
		Components:  utils.CopySlice(self.Components),
		pathspec:    pathspec,
		Manipulator: self.Manipulator,
	}
}

func (self *OSPath) SetPathSpec(pathspec *PathSpec) {
	self.Manipulator.PathParse(pathspec.Path, self)
	self.pathspec = pathspec
}

func (self *OSPath) PathSpec() *PathSpec {
	return self.Manipulator.AsPathSpec(self)
}

func (self *OSPath) DelegatePath() string {
	return self.Manipulator.AsPathSpec(self).DelegatePath
}

func (self *OSPath) DelegateAccessor() string {
	return self.Manipulator.AsPathSpec(self).DelegateAccessor
}

func (self *OSPath) Path() string {
	return self.Manipulator.AsPathSpec(self).Path
}

func (self *OSPath) String() string {
	// Cache it if we need to.
	if self.serialized != nil {
		return *self.serialized
	}

	res := self.Manipulator.PathJoin(self)
	self.serialized = &res

	return res
}

func (self *OSPath) Parse(path string) (*OSPath, error) {
	result := &OSPath{
		Manipulator: self.Manipulator,
	}

	err := self.Manipulator.PathParse(path, result)
	return result, err
}

func (self *OSPath) Basename() string {
	return self.Components[len(self.Components)-1]
}

func (self *OSPath) Dirname() *OSPath {
	result := self.Copy()
	if len(result.Components) > 0 {
		result.Components = result.Components[:len(self.Components)-1]
	}
	return result
}

func (self *OSPath) TrimComponents(components ...string) *OSPath {
	if components == nil {
		return self.Copy()
	}

	result := self.Copy()
	for idx, c := range result.Components {
		if idx >= len(components) || c != components[idx] {
			result := &OSPath{
				Components:  self.Components[idx:],
				pathspec:    self.pathspec,
				Manipulator: self.Manipulator,
			}
			return result
		}
	}
	return self
}

// Make a copy
func (self *OSPath) Append(children ...string) *OSPath {
	result := self.Copy()
	result.Components = append(result.Components, children...)

	return result
}

func (self *OSPath) Clear() *OSPath {
	return &OSPath{
		Manipulator: self.Manipulator,
	}
}

func (self *OSPath) Delegate(scope vfilter.Scope) (*OSPath, error) {
	accessor, err := GetAccessor(self.DelegateAccessor(), scope)
	if err != nil {
		return nil, err
	}

	return accessor.ParsePath(self.DelegatePath())
}

func (self *OSPath) MarshalJSON() ([]byte, error) {
	return json.Marshal(self.String())
}

// A FileInfo represents information about a file. It is similar to
// os.FileInfo but not identical.
type FileInfo interface {
	Name() string
	ModTime() time.Time

	// Path as OS serialization.
	FullPath() string

	OSPath() *OSPath

	// Time the file was birthed (initially created)
	Btime() time.Time
	Mtime() time.Time

	// Time the inode was changed.
	Ctime() time.Time
	Atime() time.Time

	// Arbitrary key/value for storing file metadata. This is accessor
	// dependent can be nil.
	Data() *ordereddict.Dict
	Size() int64

	IsDir() bool
	IsLink() bool
	GetLink() (*OSPath, error)
	Mode() os.FileMode
}

// A File reader with
type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer

	//	Stat() (FileInfo, error)
}

// Interface for accessing the filesystem.
type FileSystemAccessor interface {
	// List a directory.
	ReadDir(path string) ([]FileInfo, error)

	// Open a file for reading
	Open(path string) (ReadSeekCloser, error)
	Lstat(filename string) (FileInfo, error)

	ParsePath(filename string) (*OSPath, error)

	// The new more efficient API
	ReadDirWithOSPath(path *OSPath) ([]FileInfo, error)
	OpenWithOSPath(path *OSPath) (ReadSeekCloser, error)
	LstatWithOSPath(path *OSPath) (FileInfo, error)
	New(scope vfilter.Scope) (FileSystemAccessor, error)
}
