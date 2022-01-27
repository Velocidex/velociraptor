package glob

import (
	"fmt"

	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/vfilter"
)

type DeviceMapping struct {
	Source   string
	MapAs    string
	Type     string
	Accessor FileSystemAccessor
}

type DeviceManager struct {
	Mapping *DeviceMapping
}

func MakeNewDeviceManager(scope vfilter.Scope, remappings []*config_proto.RemappingConfig) *DeviceManager {
	if len(remappings) == 0 {
		return &DeviceManager{}
	}

	if len(remappings) > 1 {
		scope.Log("remapping: found more than 1 remapping - only applying the first one")
	}

	mapping := remappings[0]

	// if the user requested a remapping, this replaces the host's filesystem
	return &DeviceManager{
		Mapping: &DeviceMapping{
			Source: mapping.Source,
			MapAs:  "/",
			Type:   mapping.Type,
		},
	}
}

func (self DeviceManager) GetAccessor(scheme string, scope vfilter.Scope) (FileSystemAccessor, error) {
	if self.Mapping != nil {
		accessor := &remappingAccessor{
			backingAccessorType: self.Mapping.Type,
			backingFile:         self.Mapping.Source,
			scope:               scope,
		}
		if err := accessor.EnsureBackingAccessor(); err != nil {
			return nil, fmt.Errorf("cannot create backing accessor for mapping (%v)", err)
		}
		return accessor, nil
	}

	return GetAccessor(scheme, scope)
}

// WrapPath expects a raw path and wraps it into a PathSpec so that
// subsequent accessors evaluate paths properly
func (self DeviceManager) WrapPath(path string) string {
	if self.Mapping == nil {
		return path
	}

	return PathSpec{
		// this is super bad but sacrifices have to be made; forcing a file
		// accessor here basically stops us from using chained accessors in
		// remappings but for the time being this should be ok
		DelegateAccessor: "raw_file",
		DelegatePath:     self.Mapping.Source,
		Path:             path,
	}.String()
}

func GetDeviceManagerFromScope(scope vfilter.Scope) (*DeviceManager, error) {
	if manager, pres := scope.Resolve(constants.SCOPE_DEVICE_MANAGER); pres {
		return manager.(*DeviceManager), nil
	}

	return nil, errors.New("cannot retrieve device manager from scope")
}

type remappingAccessor struct {
	scope               vfilter.Scope
	backingAccessor     FileSystemAccessor
	backingAccessorType string
	backingFile         string
}

func (self remappingAccessor) New(scope vfilter.Scope) (FileSystemAccessor, error) {
	return &remappingAccessor{
		scope:               scope,
		backingAccessor:     self.backingAccessor,
		backingAccessorType: self.backingAccessorType,
		backingFile:         self.backingFile,
	}, nil
}

func (self *remappingAccessor) EnsureBackingAccessor() error {
	accessor, err := GetAccessor(self.backingAccessorType, self.scope)
	if err != nil {
		return err
	}

	self.backingAccessor = accessor
	return nil
}

func (self *remappingAccessor) ReadDir(path string) ([]FileInfo, error) {
	return self.backingAccessor.ReadDir(path)
}

func (self *remappingAccessor) Open(path string) (ReadSeekCloser, error) {
	return self.backingAccessor.Open(path)
}

func (self remappingAccessor) Lstat(filename string) (FileInfo, error) {
	return self.backingAccessor.Lstat(filename)
}

func (self *remappingAccessor) PathSplit(path string) []string {
	return self.backingAccessor.PathSplit(path)
}

func (self *remappingAccessor) PathJoin(root, stem string) string {
	return self.backingAccessor.PathJoin(root, stem)
}

func (self *remappingAccessor) GetRoot(path string) (root, subpath string, err error) {
	return self.backingAccessor.GetRoot(path)
}
