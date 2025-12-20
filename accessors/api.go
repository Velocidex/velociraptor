package accessors

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/acls"
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
	ComponentEqual(a, b string) bool
}

type OSPath struct {
	mu sync.Mutex

	Components []string

	// Some paths need more information. They store an additional path
	// spec here.
	pathspec    *PathSpec
	serialized  *string
	Manipulator PathManipulator

	// Opaque data that can be stored in the OSPath. This provides a
	// mechanism to transport additional data in the OSPath and avoid
	// having to convert back and forth.
	Data interface{}
}

func (self *OSPath) Equal(other *OSPath) bool {
	if !utils.StringSliceEq(self.Components, other.Components) {
		return false
	}

	return self.String() == other.String()
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

func (self *OSPath) SetPathSpec(pathspec *PathSpec) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	err := self.Manipulator.PathParse(pathspec.Path, self)
	if err != nil {
		return err
	}
	self.pathspec = pathspec
	return nil
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
		if idx >= len(components) ||
			!self.Manipulator.ComponentEqual(c, components[idx]) {
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

func (self *OSPath) MarshalYAML() (interface{}, error) {
	json_string := []byte(self.String())
	buf := bytes.Buffer{}
	err := json.Indent(&buf, json_string, " ", "  ")
	return string(buf.Bytes()), err
}

// MarshalText is used by the YAML marshaller. We indent the text to
// make sure it uses multi line yaml which is more readable for
// complex pathspecs.
func (self *OSPath) MarshalText() ([]byte, error) {
	json_string := []byte(self.String())
	buf := bytes.Buffer{}
	err := json.Indent(&buf, json_string, " ", "  ")
	return buf.Bytes(), err
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

// Some filesystems return multiple files with the same basename. They
// should implement this interface so we can properly dedup based on a
// unique name.
type UniqueBasename interface {
	UniqueName() string
}

// A File reader with
type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

// Some files are not really seekable (although they may pretend to
// be). Sometimes it is important to know if the file may be rewound
// back if we read from it - before we actually read from it. If a
// ReadSeekCloser also implements the Seekable interface it may report
// if it can be seeked.
type Seekable interface {
	IsSeekable() bool
}

func IsSeekable(fd ReadSeekCloser) bool {
	seekable, ok := fd.(Seekable)
	if ok {
		return seekable.IsSeekable()
	}

	return true
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

	Describe() *AccessorDescriptor
}

// Some filesystems can attempt to retrieve the underlying file. If
// this interface exists on the accessor **and** the
// GetUnderlyingAPIFilename() call succeeds, then it should be
// possible to directly access the returned filename using the OS
// APIs.
type RawFileAPIAccessor interface {
	GetUnderlyingAPIFilename(path *OSPath) (string, error)
}

var (
	NotRawFileSystem = errors.New("NotRawFileSystem")
)

func GetUnderlyingAPIFilename(accessor string,
	scope vfilter.Scope, path *OSPath) (string, error) {
	accessor_obj, err := GetAccessor(accessor, scope)
	if err != nil {
		return "", err
	}

	raw_accessor, ok := accessor_obj.(RawFileAPIAccessor)
	if !ok {
		return "", NotRawFileSystem
	}

	return raw_accessor.GetUnderlyingAPIFilename(path)
}

// For case insensitive filesystems, the canonical filename (used in
// comparisons) can be different from the actual filename.
type CanonicalFilenameAccessor interface {
	GetCanonicalFilename(path *OSPath) string
}

func GetCanonicalFilename(accessor string,
	scope vfilter.Scope, path *OSPath) string {
	accessor_obj, err := GetAccessor(accessor, scope)
	if err != nil {
		return path.String()
	}

	raw_accessor, ok := accessor_obj.(CanonicalFilenameAccessor)
	if !ok {
		return path.String()
	}

	return raw_accessor.GetCanonicalFilename(path)
}

type AccessorDescriptor struct {
	Name        string
	Description string

	// The required permissions for using this accessor
	Permissions []acls.ACL_PERMISSION

	// The name of the scope parameter that configures this accessor
	// if needed.
	ScopeVar string

	// The type description for the ScopeVar if present.
	ArgType vfilter.Any
}

func (self AccessorDescriptor) Metadata() *ordereddict.Dict {
	var permissions []string
	for _, p := range self.Permissions {
		permissions = append(permissions, p.String())
	}

	res := ordereddict.NewDict()
	if len(permissions) > 0 {
		res.Set("permissions", strings.Join(permissions, ","))
	}

	if self.ScopeVar != "" {
		res.Set("ScopeVar", self.ScopeVar)
	}

	return res
}

type DescriptorWrapper struct {
	FileSystemAccessor
	descriptor AccessorDescriptor
}

func (self DescriptorWrapper) Describe() *AccessorDescriptor {
	return &self.descriptor
}

func DescribeAccessor(target FileSystemAccessor,
	desc AccessorDescriptor) FileSystemAccessor {
	return DescriptorWrapper{
		FileSystemAccessor: target,
		descriptor:         desc,
	}
}
