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
	PathSplit(path string) []string
	PathJoin(components []string) string
}

type OSPath struct {
	Components  []string
	Manipulator PathManipulator
}

func (self OSPath) String() string {
	return self.Manipulator.PathJoin(self.Components)
}

func (self *OSPath) Parse(path string) *OSPath {
	return &OSPath{
		Components:  self.Manipulator.PathSplit(path),
		Manipulator: self.Manipulator,
	}
}

func (self *OSPath) Basename() string {
	return self.Components[len(self.Components)-1]
}

func (self *OSPath) Dirname() *OSPath {
	return &OSPath{
		Components:  utils.CopySlice(self.Components[:len(self.Components)-1]),
		Manipulator: self.Manipulator,
	}
}

func (self *OSPath) Trim(prefix *OSPath) *OSPath {
	if prefix == nil {
		return self
	}

	for idx, c := range self.Components {
		if idx >= len(prefix.Components) || c != prefix.Components[idx] {
			return &OSPath{
				Components:  utils.CopySlice(self.Components[idx:]),
				Manipulator: self.Manipulator,
			}
		}
	}
	return self
}

func (self *OSPath) Append(children ...string) *OSPath {
	return &OSPath{
		Components:  utils.CopySlice(append(self.Components, children...)),
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
}

// A factory for new accessors
type FileSystemAccessorFactory interface {
	New(scope vfilter.Scope) (FileSystemAccessor, error)
}
