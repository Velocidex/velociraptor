package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type ScopeFilesystemAccessor struct {
	scope vfilter.Scope
}

func (self ScopeFilesystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
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

func (self ScopeFilesystemAccessor) Lstat(variable string) (glob.FileInfo, error) {
	str, err := self.getData(variable)
	if err != nil {
		return nil, err
	}
	return utils.NewDataFileInfo(str), err
}

func (self ScopeFilesystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self ScopeFilesystemAccessor) Open(path string) (glob.ReadSeekCloser, error) {
	str, err := self.getData(path)
	if err != nil {
		return nil, err
	}
	return utils.DataReadSeekCloser{
		ReadSeeker: strings.NewReader(str),
		Data:       path,
	}, nil
}

func (self ScopeFilesystemAccessor) PathSplit(path string) []string {
	return paths.GenericPathSplit(path)
}

func (self ScopeFilesystemAccessor) PathJoin(root, stem string) string {
	return filepath.Join(root, stem)
}

func (self ScopeFilesystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func init() {
	glob.Register("scope", &ScopeFilesystemAccessor{}, `Similar to the "data" accessor, this makes a string appears as a file. However, instead of the Filename containing the file content itself, the Filename refers to the name of a variable in the current scope that contains the data. This is useful when the binary data is not unicode safe and can not be properly represented by JSON.`)
}
