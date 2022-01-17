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

func getDescription(accessorName string, scope vfilter.Scope) string {
	if desc, pres := DescribeAccessors().Get(accessorName); pres {
		return desc.(string)
	}

	return ""
}

func (self DeviceManager) InjectRemapping(scope vfilter.Scope) {
	if self.Mapping == nil {
		return
	}

	if _, err := GetAccessor("file", scope); err == nil {
		accessor := remappingAccessor{
			pathSpec: &PathSpec{
				DelegateAccessor: self.Mapping.Type,
				DelegatePath:     self.Mapping.Source,
			},
		}

		if err := accessor.EnsureBackingAccessor(); err != nil {
			scope.Log("remapping: cannot re-register file accessor (%v)", err)
		} else {
			Register("file", accessor, getDescription("file", scope))
		}
	}

	if _, err := GetAccessor("raw_file", scope); err == nil {
		accessor := remappingAccessor{
			pathSpec: &PathSpec{
				DelegateAccessor: self.Mapping.Type,
				DelegatePath:     self.Mapping.Source,
			},
		}

		if err := accessor.EnsureBackingAccessor(); err != nil {
			scope.Log("remapping: cannot re-register raw_file accessor (%v)", err)
		} else {
			Register("raw_file", accessor, getDescription("raw_file", scope))
		}
	}
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
		pathSpec := &PathSpec{
			DelegateAccessor: self.Mapping.Type,
			DelegatePath:     self.Mapping.Source,
		}
		accessor := &remappingAccessor{
			scope:    scope,
			pathSpec: pathSpec,
		}
		if err := accessor.EnsureBackingAccessor(); err != nil {
			return nil, fmt.Errorf("cannot create backing accessor for mapping (%v)", err)
		}
		return accessor, nil
	}

	return GetAccessor(scheme, scope)
}

func GetDeviceManagerFromScope(scope vfilter.Scope) (*DeviceManager, error) {
	if manager, pres := scope.Resolve(constants.SCOPE_DEVICE_MANAGER); pres {
		return manager.(*DeviceManager), nil
	}

	return nil, errors.New("cannot retrieve device manager from scope")
}

type remappingAccessor struct {
	scope           vfilter.Scope
	pathSpec        *PathSpec
	backingAccessor FileSystemAccessor
}

func (self remappingAccessor) New(scope vfilter.Scope) (FileSystemAccessor, error) {
	return &remappingAccessor{scope: scope, pathSpec: self.pathSpec, backingAccessor: self.backingAccessor}, nil
}

func (self *remappingAccessor) EnsureBackingAccessor() error {
	accessor, err := GetAccessor(self.pathSpec.DelegateAccessor, self.scope)
	if err != nil {
		return err
	}

	self.backingAccessor = accessor
	return nil
}

func (self *remappingAccessor) ReadDir(path string) ([]FileInfo, error) {
	pathSpec, err := PathSpecFromString(path)
	if err != nil {
		return nil, err
	}

	spec := *self.pathSpec
	spec.Path = pathSpec.DelegatePath

	return self.backingAccessor.ReadDir(spec.String())
}

func (self *remappingAccessor) Open(path string) (ReadSeekCloser, error) {
	pathSpec, err := PathSpecFromString(path)
	if err != nil {
		return nil, err
	}

	spec := *self.pathSpec
	spec.Path = pathSpec.DelegatePath

	return self.backingAccessor.Open(spec.String())
}

func (self remappingAccessor) Lstat(filename string) (FileInfo, error) {
	pathSpec, err := PathSpecFromString(filename)
	if err != nil {
		return nil, err
	}

	spec := *self.pathSpec
	spec.Path = pathSpec.DelegatePath

	return self.backingAccessor.Lstat(spec.String())
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
