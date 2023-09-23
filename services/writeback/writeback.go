package writeback

import (
	"errors"
	"os"
	"runtime"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	writeback_not_enabled_error = errors.New("Writeback not enabled")
)

type WritebackService struct {
	mu         sync.Mutex
	dispatcher map[string]*WritebackManager
}

func (self *WritebackService) MutateWriteback(
	config_obj *config_proto.Config,
	cb func(wb *config_proto.Writeback) error) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	location, err := self.WritebackLocation(config_obj)
	if err != nil {
		return err
	}

	manager, pres := self.dispatcher[location]
	if !pres {
		return writeback_not_enabled_error
	}

	return manager.MutateWriteback(cb)
}

func (self *WritebackService) GetWriteback(
	config_obj *config_proto.Config) (
	*config_proto.Writeback, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	location, err := self.WritebackLocation(config_obj)
	if err != nil {
		return nil, err
	}

	manager, pres := self.dispatcher[location]
	if !pres {
		return nil, writeback_not_enabled_error
	}

	return manager.GetWriteback(), nil
}

func (self *WritebackService) LoadWriteback(
	config_obj *config_proto.Config) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	location, err := self.WritebackLocation(config_obj)
	if err != nil {
		return writeback_not_enabled_error
	}

	manager, pres := self.dispatcher[location]
	if !pres {
		manager = NewWritebackManager(config_obj, location)
		self.dispatcher[location] = manager
	}

	return manager.Load()
}

// Return the location of the writeback file.
func (self *WritebackService) WritebackLocation(
	config_obj *config_proto.Config) (string, error) {
	if config_obj == nil || config_obj.Client == nil {
		return "", errors.New("Client not configured")
	}

	switch runtime.GOOS {
	case "darwin":
		return os.ExpandEnv(config_obj.Client.WritebackDarwin), nil

	case "linux":
		return os.ExpandEnv(config_obj.Client.WritebackLinux), nil

	case "windows":
		return os.ExpandEnv(config_obj.Client.WritebackWindows), nil

	default:
		return os.ExpandEnv(config_obj.Client.WritebackLinux), nil
	}
}

func NewWritebackService() WritebackServiceInterface {
	return &WritebackService{
		dispatcher: make(map[string]*WritebackManager),
	}
}
