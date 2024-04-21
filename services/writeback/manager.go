package writeback

import (
	"sync"

	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type WritebackManager struct {
	mu         sync.Mutex
	config_obj *config_proto.Config
	store      WritebackStorer

	writeback *config_proto.Writeback
}

func (self *WritebackManager) MutateWriteback(
	cb func(wb *config_proto.Writeback) error) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	cb_err := cb(self.writeback)
	if cb_err == WritebackNoUpdate {
		return nil
	}

	if cb_err == WritebackUpdateLevel1 {
		return self.store.WriteL1(self.writeback)
	}

	if cb_err == nil || cb_err == WritebackUpdateLevel2 {
		return self.store.WriteL2(self.writeback)
	}

	return cb_err
}

func (self *WritebackManager) GetWriteback() *config_proto.Writeback {
	return proto.Clone(self.writeback).(*config_proto.Writeback)
}

// Load the writeback from the disk
func (self *WritebackManager) Load() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.writeback = self.store.Load()
	return nil
}

func NewWritebackManager(
	config_obj *config_proto.Config,
	location string) *WritebackManager {
	return &WritebackManager{
		config_obj: config_obj,
		store:      GetFileWritebackStore(config_obj),
		writeback:  &config_proto.Writeback{},
	}
}
