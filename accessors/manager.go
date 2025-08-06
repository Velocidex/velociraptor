package accessors

import (
	"fmt"
	"sync"

	errors "github.com/go-errors/errors"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	mu sync.Mutex

	// A global device manager is used to register handles.
	globalDeviceManager *DefaultDeviceManager = NewDefaultDeviceManager()
)

// A device manager is a factory for creating accessors.
type DeviceManager interface {
	GetAccessor(scheme string, scope vfilter.Scope) (FileSystemAccessor, error)
	Copy() DeviceManager
	Clear()
	Register(accessor FileSystemAccessor)
}

func GetManager(scope vfilter.Scope) DeviceManager {
	manager_any, pres := scope.Resolve(constants.SCOPE_DEVICE_MANAGER)
	if pres {
		manager, ok := manager_any.(DeviceManager)
		if ok {
			return manager
		}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return globalDeviceManager.Copy()
	}

	return GetDefaultDeviceManager(config_obj)
}

func GetDefaultDeviceManager(config_obj *config_proto.Config) DeviceManager {
	mu.Lock()
	defer mu.Unlock()

	return globalDeviceManager
}

func GetAccessor(scheme string, scope vfilter.Scope) (FileSystemAccessor, error) {
	// Fallback to the file handler - this should work
	// because there needs to be at least a file handler
	// registered.
	switch scheme {

	case "":
		scheme = "auto"

	case "reg":
		// Backwards compatibility uses old shortname for reg
		// accessor.
		scheme = "registry"
	}

	return GetManager(scope).GetAccessor(scheme, scope)
}

// The default device manager is global and uses the
type DefaultDeviceManager struct {
	mu       sync.Mutex
	handlers map[string]FileSystemAccessor
}

func NewDefaultDeviceManager() *DefaultDeviceManager {
	return &DefaultDeviceManager{
		handlers: make(map[string]FileSystemAccessor),
	}
}

func (self *DefaultDeviceManager) GetAccessor(
	scheme string, scope vfilter.Scope) (FileSystemAccessor, error) {

	self.mu.Lock()
	handler, pres := self.handlers[scheme]
	self.mu.Unlock()

	if pres {
		// Check permissions for accessing this handler.
		for _, p := range handler.Describe().Permissions {
			err := vql_subsystem.CheckAccess(scope, p)
			if err != nil {
				return nil, fmt.Errorf("Accessor %v: %w", scheme, err)
			}
		}

		res, err := handler.New(scope)
		return res, err
	}
	return nil, errors.New("Unknown filesystem accessor " + scheme)
}

func (self *DefaultDeviceManager) Register(accessor FileSystemAccessor) {
	self.mu.Lock()
	defer self.mu.Unlock()

	desc := accessor.Describe()
	self.handlers[desc.Name] = accessor
}

func (self *DefaultDeviceManager) DescribeAccessors() (res []*AccessorDescriptor) {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, h := range self.handlers {
		res = append(res, h.Describe())
	}
	return res
}

func (self *DefaultDeviceManager) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.handlers = make(map[string]FileSystemAccessor)
}

func (self *DefaultDeviceManager) Copy() DeviceManager {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.copy()
}

func (self *DefaultDeviceManager) copy() *DefaultDeviceManager {
	result := NewDefaultDeviceManager()
	for k, v := range self.handlers {
		result.handlers[k] = v
	}
	return result
}

func Register(accessor FileSystemAccessor) {
	globalDeviceManager.Register(accessor)
}

func DescribeAccessors() []*AccessorDescriptor {
	return globalDeviceManager.DescribeAccessors()
}
