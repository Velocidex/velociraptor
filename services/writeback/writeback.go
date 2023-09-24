package writeback

import (
	"errors"
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

	location, err := WritebackLocation(config_obj)
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

	location, err := WritebackLocation(config_obj)
	if err != nil {
		return nil, err
	}

	manager, pres := self.dispatcher[location]
	if !pres {
		return nil, writeback_not_enabled_error
	}

	return manager.GetWriteback(), nil
}

// Only used in a test
func (self *WritebackService) Reset(
	config_obj *config_proto.Config) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	location, err := WritebackLocation(config_obj)
	if err != nil {
		return writeback_not_enabled_error
	}

	manager := NewWritebackManager(config_obj, location)
	self.dispatcher[location] = manager
	return manager.Load()
}

func (self *WritebackService) LoadWriteback(
	config_obj *config_proto.Config) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	location, err := WritebackLocation(config_obj)
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

func NewWritebackService() WritebackServiceInterface {
	return &WritebackService{
		dispatcher: make(map[string]*WritebackManager),
	}
}
