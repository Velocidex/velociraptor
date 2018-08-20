package glob

import (
	"regexp"
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
}

func GetAccessor(scheme string) FileSystemAccessor {
	handler, pres := handlers[scheme]
	if pres {
		return handler
	}

	// Fallback to the file handler - this should work
	// because there needs to be at least a file handler
	// registered.
	handler, pres = handlers["file"]
	if pres {
		return handler
	}

	panic("There must be at least a file handler registered")
}

func Register(scheme string, accessor FileSystemAccessor) {
	if handlers == nil {
		handlers = make(map[string]FileSystemAccessor)
	}

	handlers[scheme] = accessor
}
