package glob

import (
	"context"
	"regexp"

	errors "github.com/pkg/errors"
)

var (
	handlers map[string]FileSystemAccessor
)

// Interface for accessing the filesystem. Used for dependency
// injection.
type FileSystemAccessor interface {
	ReadDir(path string) ([]FileInfo, error)
	Open(path string) (ReadSeekCloser, error)
	Lstat(filename string) (FileInfo, error)

	// Produce a regex which can be used to split the path into
	// components.
	PathSplit() *regexp.Regexp

	// The most natural way to join paths together.
	PathSep() string

	// Split a path into a glob root and a sub path
	// component. This is required when the accessor uses a prefix
	// which contains path separators but should not be considered
	// as part of the glob. For example, the ntfs accessor uses a
	// device path as the root, which already contains path
	// separators:
	// \\.\c:\Windows\System32\notepad.exe ->
	// root = \\.\c:
	// path = \Windows\System32\notepad.exe
	GetRoot(path string) (root, subpath string, err error)

	// A factory for new accessors
	New(ctx context.Context) FileSystemAccessor
}

type NullFileSystemAccessor struct{}

func (self NullFileSystemAccessor) New(ctx context.Context) FileSystemAccessor {
	return self
}

func (self NullFileSystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	return nil, errors.New("Not supported")
}

func (self NullFileSystemAccessor) Open(path string) (ReadSeekCloser, error) {
	return nil, errors.New("Not supported")
}

func (self NullFileSystemAccessor) Lstat(path string) (FileInfo, error) {
	return nil, errors.New("Not supported")
}

func (self NullFileSystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func (self NullFileSystemAccessor) PathSplit() *regexp.Regexp {
	return regexp.MustCompile("/")
}

func (self NullFileSystemAccessor) PathSep() string {
	return "/"
}

func GetAccessor(scheme string, ctx context.Context) FileSystemAccessor {
	handler, pres := handlers[scheme]
	if pres {
		return handler.New(ctx)
	}

	// Fallback to the file handler - this should work
	// because there needs to be at least a file handler
	// registered.
	if scheme == "" {
		handler, pres = handlers["file"]
		if pres {
			return handler.New(ctx)
		}
	}

	return NullFileSystemAccessor{}
}

func Register(scheme string, accessor FileSystemAccessor) {
	if handlers == nil {
		handlers = make(map[string]FileSystemAccessor)
	}

	handlers[scheme] = accessor
}
