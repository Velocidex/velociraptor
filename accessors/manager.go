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
	Register(scheme string, accessor FileSystemAccessorFactory, description string)
}

func GetAccessor(scheme string, scope vfilter.Scope) (FileSystemAccessor, error) {
	// Fallback to the file handler - this should work
	// because there needs to be at least a file handler
	// registered.
	if scheme == "" {
		scheme = "file"
	}

	manager_any, pres := scope.Resolve(constants.SCOPE_DEVICE_MANAGER)
	if pres {
		manager, ok := manager_any.(DeviceManager)
		if ok {
			return manager.GetAccessor(scheme, scope)
		}
	}

	return GlobalDeviceManager.GetAccessor(scheme, scope)
}

// The default device manager is global and uses the
type DefaultDeviceManager struct {
	mu           sync.Mutex
	handlers     map[string]FileSystemAccessorFactory
	descriptions *ordereddict.Dict
}

func NewDefaultDeviceManager() *DefaultDeviceManager {
	return &DefaultDeviceManager{
		handlers:     make(map[string]FileSystemAccessorFactory),
		descriptions: ordereddict.NewDict(),
	}
}

func (self *DefaultDeviceManager) GetAccessor(
	scheme string, scope vfilter.Scope) (FileSystemAccessor, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	handler, pres := self.handlers[scheme]
	if pres {
		res, err := handler.New(scope)
		return res, err
	}
	scope.Log("Unknown filesystem accessor: " + scheme)
	return nil, errors.New("Unknown filesystem accessor " + scheme)
}

func (self *DefaultDeviceManager) Register(
	scheme string, accessor FileSystemAccessorFactory, description string) {
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

func (self *DefaultDeviceManager) Copy() *DefaultDeviceManager {
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
	scheme string, accessor FileSystemAccessorFactory, description string) {
	GlobalDeviceManager.Register(scheme, accessor, description)
}

func DescribeAccessors() *ordereddict.Dict {
	return GlobalDeviceManager.DescribeAccessors()
}
