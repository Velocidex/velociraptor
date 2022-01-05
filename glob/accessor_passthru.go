package glob

import (
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/vfilter"
)

type PassthruAccessor struct {
	backingAccessor FileSystemAccessor
	raw_file        bool
}

func (self PassthruAccessor) New(scope vfilter.Scope) (FileSystemAccessor, error) {
	if backingAccessor, ok := scope.Resolve(constants.SCOPE_BACKING_ACCESSOR); ok {
		self.backingAccessor = backingAccessor.(FileSystemAccessor)
	} else {
		accessorType := "os_file"
		if self.raw_file {
			accessorType = "os_raw_file"
		}

		accessor, err := GetAccessor(accessorType, scope)
		if err != nil {
			return nil, err
		}

		self.backingAccessor = accessor
	}

	return self, nil
}

func (self PassthruAccessor) ReadDir(path string) ([]FileInfo, error) {
	return self.backingAccessor.ReadDir(path)
}

func (self PassthruAccessor) Open(path string) (ReadSeekCloser, error) {
	return self.backingAccessor.Open(path)
}

func (self PassthruAccessor) Lstat(filename string) (FileInfo, error) {
	return self.backingAccessor.Lstat(filename)
}

func (self PassthruAccessor) PathSplit(path string) []string {
	return self.backingAccessor.PathSplit(path)
}
func (self PassthruAccessor) PathJoin(root, stem string) string {
	return self.backingAccessor.PathJoin(root, stem)
}
func (self PassthruAccessor) GetRoot(path string) (root, subpath string, err error) {
	return self.backingAccessor.GetRoot(path)
}

func init() {
	Register("file", &PassthruAccessor{}, `Access files using any backing accessor. Falls back to os_file if none provided.`)
	Register("raw_file", &PassthruAccessor{raw_file: true}, `Access files using any backing accessor. Falls back to os_raw_file if none provided.`)
}
