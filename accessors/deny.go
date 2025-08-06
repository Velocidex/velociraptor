package accessors

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

var (
	NotImplementedError = utils.NotImplementedError
)

type UnimplementedAccessor struct {
	Name string
}

func (self UnimplementedAccessor) ReadDir(path string) ([]FileInfo, error) {
	return nil, fmt.Errorf("%v: %w: Accessor denied by configuration",
		self.Name, NotImplementedError)
}

func (self UnimplementedAccessor) Open(path string) (ReadSeekCloser, error) {
	return nil, fmt.Errorf("%v: %w: Accessor denied by configuration",
		self.Name, NotImplementedError)
}

func (self UnimplementedAccessor) Lstat(filename string) (FileInfo, error) {
	return nil, fmt.Errorf("%v: %w: Accessor denied by configuration",
		self.Name, NotImplementedError)
}

func (self UnimplementedAccessor) ParsePath(filename string) (*OSPath, error) {
	return nil, fmt.Errorf("%v: %w: Accessor denied by configuration",
		self.Name, NotImplementedError)
}

func (self UnimplementedAccessor) ReadDirWithOSPath(path *OSPath) ([]FileInfo, error) {
	return nil, fmt.Errorf("%v: %w: Accessor denied by configuration",
		self.Name, NotImplementedError)
}

func (self UnimplementedAccessor) OpenWithOSPath(path *OSPath) (ReadSeekCloser, error) {
	return nil, fmt.Errorf("%v: %w: Accessor denied by configuration",
		self.Name, NotImplementedError)
}

func (self UnimplementedAccessor) LstatWithOSPath(path *OSPath) (FileInfo, error) {
	return nil, fmt.Errorf("%v: %w: Accessor denied by configuration",
		self.Name, NotImplementedError)
}

func (self UnimplementedAccessor) New(scope vfilter.Scope) (FileSystemAccessor, error) {
	return nil, fmt.Errorf("%v: %w: Accessor denied by configuration",
		self.Name, NotImplementedError)
}

func (self UnimplementedAccessor) Describe() *AccessorDescriptor {
	return &AccessorDescriptor{
		Name:        self.Name,
		Description: "Blocked accessor",
	}
}

func EnforceAccessorAllowList(
	allowed_accessors []string, deny_accessors []string) error {
	mu.Lock()
	defer mu.Unlock()

	global_manager := globalDeviceManager

	if len(allowed_accessors) > 0 {
		globalDeviceManager = NewDefaultDeviceManager()

		for _, allowed := range allowed_accessors {
			impl, ok := global_manager.handlers[allowed]
			if !ok {
				return fmt.Errorf("Unknown accessor in allow list: %v", allowed)
			}

			globalDeviceManager.handlers[allowed] = impl
		}
	}

	for _, deny := range deny_accessors {
		globalDeviceManager.Register(&UnimplementedAccessor{
			Name: deny,
		})
	}

	return nil
}
