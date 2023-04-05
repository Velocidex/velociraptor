package accessors

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
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
	mu sync.Mutex

	Components []string

	// Some paths need more information. They store an additional path
	// spec here.
	pathspec    *PathSpec
	serialized  *string
	Manipulator PathManipulator
}

func (self *OSPath) DescribeType() string {
	subtype := ""
	switch self.Manipulator.(type) {
	case LinuxPathManipulator:
		subtype = "LinuxPath"
	case GenericPathManipulator:
		subtype = "Generic"
	case WindowsPathManipulator:
		subtype = "WindowsPath"
	case WindowsNTFSManipulator:
		subtype = "NTFSPath"
	case WindowsRegistryPathManipulator:
		subtype = "RegistryPath"
	case PathSpecPathManipulator:
		subtype = "PathSpec"
	case FileStorePathManipulator:
		subtype = "FileStorePath"
	case RawFileManipulator:
		subtype = "RawPath"
	case ZipFileManipulator:
		subtype = "ZipPathspec"
	default:
		subtype = fmt.Sprintf("%T", self.Manipulator)
	}
	return fmt.Sprintf("OSPath(%s)", subtype)
}

// Make a copy of the OSPath
func (self *OSPath) Copy() *OSPath {
	self.mu.Lock()
	defer self.mu.Unlock()

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
	self.mu.Lock()
	defer self.mu.Unlock()

	self.Manipulator.PathParse(pathspec.Path, self)
	self.pathspec = pathspec
}

func (self *OSPath) PathSpec() *PathSpec {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.Manipulator.AsPathSpec(self)
}

func (self *OSPath) DelegatePath() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	pathspec := self.Manipulator.AsPathSpec(self)
	if pathspec.DelegatePath == "" && pathspec.Delegate != nil {
		pathspec.DelegatePath = json.MustMarshalString(pathspec.Delegate)
	}
	return pathspec.DelegatePath
}

func (self *OSPath) DelegateAccessor() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.Manipulator.AsPathSpec(self).DelegateAccessor
}

func (self *OSPath) Path() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.Manipulator.AsPathSpec(self).Path
}

func (self *OSPath) String() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Cache it if we need to.
	if self.serialized != nil {
		return *self.serialized
	}

	res := self.Manipulator.PathJoin(self)
	self.serialized = &res

	return res
}

func (self *OSPath) Parse(path string) (*OSPath, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := &OSPath{
		Manipulator: self.Manipulator,
	}

	err := self.Manipulator.PathParse(path, result)
	return result, err
}

func (self *OSPath) Basename() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	if len(self.Components) > 0 {
		return self.Components[len(self.Components)-1]
	}
	return ""
}

func (self *OSPath) Dirname() *OSPath {
	result := self.Copy()
	if len(result.Components) > 0 {
		result.Components = result.Components[:len(self.Components)-1]
	}
	return result
}

// TrimComponents removes the specified components from the start of
// our own components.
// For example if self = ["C:", "Windows", "System32"]
// then TrimComponents("C:") -> ["Windows", "System32"]
func (self *OSPath) TrimComponents(components ...string) *OSPath {
	if components == nil {
		return self.Copy()
	}

	result := self.Copy()
	for idx, c := range result.Components {
		if idx >= len(components) || c != components[idx] {
			result := &OSPath{
				Components:  utils.CopySlice(self.Components[idx:]),
				pathspec:    self.pathspec,
				Manipulator: self.Manipulator,
			}
			return result
		}
	}
	result.Components = nil
	return result
}

// Produce a human readable string - this is a one way conversion: It
// is not possible to go back to a proper OSPath from this.
func (self *OSPath) HumanString(scope types.Scope) string {
	result := []string{self.Path()}
	delegate := self

	for {
		next_delegate, err := delegate.Delegate(scope)
		if err != nil {
			break
		}
		delegate = next_delegate
		if len(delegate.Components) == 0 {
			break
		}
		result = append(result, delegate.Path())
	}

	// Reverse the slice
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	// As an alternative form we can use maybe? return path.Join(result...)
	return strings.Join(result, " -> ")
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
}

// Interface for accessing the filesystem.
type FileSystemAccessor interface {
	// List a directory.
	ReadDir(path string) ([]FileInfo, error)

	// Open a file for reading
	Open(path string) (ReadSeekCloser, error)
	Lstat(filename string) (FileInfo, error)

	// Converts from a string path to an OSPath suitable for this
	// accessor.
	ParsePath(filename string) (*OSPath, error)

	// The new more efficient API
	ReadDirWithOSPath(path *OSPath) ([]FileInfo, error)
	OpenWithOSPath(path *OSPath) (ReadSeekCloser, error)
	LstatWithOSPath(path *OSPath) (FileInfo, error)
	New(scope vfilter.Scope) (FileSystemAccessor, error)
}
