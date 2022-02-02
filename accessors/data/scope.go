package data

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/vfilter"
)

type ScopeFilesystemAccessor struct {
	scope vfilter.Scope
}

func (self ScopeFilesystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	return ScopeFilesystemAccessor{scope}, nil
}

func (self ScopeFilesystemAccessor) getData(variable string) (string, error) {
	variable_data, pres := self.scope.Resolve(variable)
	if !pres {
		return "", os.ErrNotExist
	}

	switch t := variable_data.(type) {
	case string:
		return t, nil

	case []byte:
		return string(t), nil

	default:
		return fmt.Sprintf("%v", variable_data), nil
	}
}

func (self ScopeFilesystemAccessor) ParsePath(path string) *accessors.OSPath {
	return accessors.NewLinuxOSPath(path)
}

func (self ScopeFilesystemAccessor) Lstat(variable string) (
	accessors.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self ScopeFilesystemAccessor) ReadDir(path string) (
	[]accessors.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self ScopeFilesystemAccessor) Open(path string) (
	accessors.ReadSeekCloser, error) {
	str, err := self.getData(path)
	if err != nil {
		return nil, err
	}
	return accessors.VirtualReadSeekCloser{
		ReadSeeker: strings.NewReader(str),
	}, nil
}

func init() {
	accessors.Register("scope", &ScopeFilesystemAccessor{},
		`Similar to the "data" accessor, this makes a string appears as a file. However, instead of the Filename containing the file content itself, the Filename refers to the name of a variable in the current scope that contains the data. This is useful when the binary data is not unicode safe and can not be properly represented by JSON.`)
}
