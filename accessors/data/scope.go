package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type ScopeFilesystemAccessor struct {
	scope vfilter.Scope
}

func (self ScopeFilesystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "scope",
		Description: `Present the content of a scope variable as a file.`,
	}
}

func (self ScopeFilesystemAccessor) New(scope vfilter.Scope) (
	accessors.FileSystemAccessor, error) {
	return ScopeFilesystemAccessor{scope}, nil
}

func (self ScopeFilesystemAccessor) getData(variable string) (string, error) {
	var result vfilter.Any = self.scope
	var pres bool

	for _, member := range strings.Split(variable, ".") {
		switch t := result.(type) {
		case types.LazyExpr:
			result = t.Reduce(context.Background())
		}
		result, pres = self.scope.Associative(result, member)
		if !pres {
			return "", utils.NotFoundError
		}
	}

	switch t := result.(type) {
	case string:
		return t, nil

	case []byte:
		return string(t), nil

	default:
		return fmt.Sprintf("%v", result), nil
	}
}

func (self ScopeFilesystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.MustNewPathspecOSPath("").Clear().Append(path), nil
}

func (self ScopeFilesystemAccessor) LstatWithOSPath(path *accessors.OSPath) (
	accessors.FileInfo, error) {
	if len(path.Components) != 1 {
		return nil, utils.NotFoundError
	}

	return self.Lstat(path.Components[0])
}

func (self ScopeFilesystemAccessor) Lstat(variable string) (
	accessors.FileInfo, error) {
	str, err := self.getData(variable)
	if err != nil {
		return nil, err
	}

	full_path, err := self.ParsePath(variable)
	if err != nil {
		return nil, err
	}

	return &accessors.VirtualFileInfo{
		RawData: []byte(str),
		Path:    full_path,
	}, nil
}

func (self ScopeFilesystemAccessor) ReadDir(path string) (
	[]accessors.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self ScopeFilesystemAccessor) ReadDirWithOSPath(path *accessors.OSPath) (
	[]accessors.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self ScopeFilesystemAccessor) OpenWithOSPath(path *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {
	if len(path.Components) != 1 {
		return nil, utils.NotFoundError
	}

	return self.Open(path.Components[0])
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
	accessors.Register(&ScopeFilesystemAccessor{})
}
