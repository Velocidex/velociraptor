package accessors

import (
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
	PathParse(path string, result *OSPath)
	PathJoin(path *OSPath) string
	AsPathSpec(path *OSPath) *PathSpec
}

type OSPath struct {
	Components []string

	// Some paths need more information. They store an additional path
	// spec here.
	pathspec    *PathSpec
	Manipulator PathManipulator
}

func (self OSPath) Copy() *OSPath {
	self.pathspec = self.pathspec.Copy()
	return &self
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
	return self.Manipulator.PathJoin(self)
}

func (self *OSPath) Parse(path string) *OSPath {
	result := &OSPath{
		Manipulator: self.Manipulator,
	}

	self.Manipulator.PathParse(path, result)

	return result
}

func (self *OSPath) Basename() string {
	return self.Components[len(self.Components)-1]
}

func (self *OSPath) Dirname() *OSPath {
	return &OSPath{
		Components:  utils.CopySlice(self.Components[:len(self.Components)-1]),
		pathspec:    self.pathspec,
		Manipulator: self.Manipulator,
	}
}

func (self *OSPath) TrimComponents(components ...string) *OSPath {
	if components == nil {
		return self
	}

	for idx, c := range self.Components {
		if idx >= len(components) || c != components[idx] {
			result := &OSPath{
				Components:  utils.CopySlice(self.Components[idx:]),
				pathspec:    self.pathspec,
				Manipulator: self.Manipulator,
			}
			return result
		}
	}
	return self
}

func (self *OSPath) Append(children ...string) *OSPath {
	return &OSPath{
		Components:  utils.CopySlice(append(self.Components, children...)),
		pathspec:    self.pathspec,
		Manipulator: self.Manipulator,
	}
}

func (self *OSPath) Clear() *OSPath {
	return &OSPath{
		Manipulator: self.Manipulator,
	}
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

	ParsePath(filename string) *OSPath
}

// A factory for new accessors
type FileSystemAccessorFactory interface {
	New(scope vfilter.Scope) (FileSystemAccessor, error)
}
