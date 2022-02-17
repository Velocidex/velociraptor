package accessors

import (
	"sync"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/vfilter"
)

var (
	GlobalDeviceManager = NewDefaultDeviceManager()
)

// A device manager is a factory for creating accessors.
type DeviceManager interface {
	GetAccessor(scheme string, scope vfilter.Scope) (FileSystemAccessor, error)
	Copy() DeviceManager
	Clear()
	Register(scheme string, accessor FileSystemAccessor, description string)
}

func GetManager(scope vfilter.Scope) DeviceManager {
	manager_any, pres := scope.Resolve(constants.SCOPE_DEVICE_MANAGER)
	if pres {
		manager, ok := manager_any.(DeviceManager)
		if ok {
			return manager
		}
	}

	return GlobalDeviceManager
}

func GetAccessor(scheme string, scope vfilter.Scope) (FileSystemAccessor, error) {
	// Fallback to the file handler - this should work
	// because there needs to be at least a file handler
	// registered.
	if scheme == "" {
		scheme = "auto"
	}

	return GetManager(scope).GetAccessor(scheme, scope)
}

// The default device manager is global and uses the
type DefaultDeviceManager struct {
	mu           sync.Mutex
	handlers     map[string]FileSystemAccessor
	descriptions *ordereddict.Dict
}

func NewDefaultDeviceManager() *DefaultDeviceManager {
	return &DefaultDeviceManager{
		handlers:     make(map[string]FileSystemAccessor),
		descriptions: ordereddict.NewDict(),
	}
}

func (self *DefaultDeviceManager) GetAccessor(
	scheme string, scope vfilter.Scope) (FileSystemAccessor, error) {

	self.mu.Lock()
	handler, pres := self.handlers[scheme]
	self.mu.Unlock()

	if pres {
		res, err := handler.New(scope)
		return res, err
	}
	return nil, errors.New("Unknown filesystem accessor " + scheme)
}

func (self *DefaultDeviceManager) Register(
	scheme string, accessor FileSystemAccessor, description string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.handlers[scheme] = accessor
	self.descriptions.Set(scheme, description)
}

func (self *DefaultDeviceManager) DescribeAccessors() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.descriptions
}

func (self *DefaultDeviceManager) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.handlers = make(map[string]FileSystemAccessor)
	self.descriptions = ordereddict.NewDict()
}

func (self *DefaultDeviceManager) Copy() DeviceManager {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := NewDefaultDeviceManager()
	for k, v := range self.handlers {
		result.handlers[k] = v
	}

	result.descriptions = ordereddict.NewDict()
	result.descriptions.MergeFrom(self.descriptions)
	return result
}

func Register(
	scheme string, accessor FileSystemAccessor, description string) {
	GlobalDeviceManager.Register(scheme, accessor, description)
}

func DescribeAccessors() *ordereddict.Dict {
	return GlobalDeviceManager.DescribeAccessors()
}
