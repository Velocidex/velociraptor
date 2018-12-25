// A data filesystem accessor - allows data to be read as a file.

package glob

import (
	"context"
	"regexp"
	"strings"

	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/utils"
)

type DataFilesystemAccessor struct{}

func (self DataFilesystemAccessor) New(ctx context.Context) FileSystemAccessor {
	return DataFilesystemAccessor{}
}

func (self DataFilesystemAccessor) Lstat(filename string) (FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self DataFilesystemAccessor) ReadDir(path string) ([]FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self DataFilesystemAccessor) Open(path string) (ReadSeekCloser, error) {
	return utils.DataReadSeekCloser{strings.NewReader(path)}, nil
}

func (self DataFilesystemAccessor) PathSplit() *regexp.Regexp {
	return regexp.MustCompile("/")
}

func (self DataFilesystemAccessor) PathSep() string {
	return "/"
}

func (self DataFilesystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func init() {
	Register("data", &DataFilesystemAccessor{})
}
