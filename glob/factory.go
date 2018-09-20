package glob

import (
	"context"
	"regexp"

	errors "github.com/pkg/errors"
)

var (
	scheme_regex = regexp.MustCompile("^([a-z]+):(.+)$")
	handlers     map[string]FileSystemAccessor
)

// Interface for accessing the filesystem. Used for dependency
// injection.
type FileSystemAccessor interface {
	ReadDir(path string) ([]FileInfo, error)
	Open(path string) (ReadSeekCloser, error)
	Lstat(filename string) (FileInfo, error)
	PathSep() *regexp.Regexp

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

func (self NullFileSystemAccessor) PathSep() *regexp.Regexp {
	return regexp.MustCompile("/")
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
