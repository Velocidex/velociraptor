package writeback

import (
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/Velocidex/yaml/v2"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type WritebackManager struct {
	mu         sync.Mutex
	config_obj *config_proto.Config
	location   string
	writeback  *config_proto.Writeback

	dirty bool
}

func (self *WritebackManager) MutateWriteback(
	cb func(wb *config_proto.Writeback) error) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	cb_err := cb(self.writeback)
	if cb_err == WritebackNoUpdate {
		return nil
	}

	bytes, err := yaml.Marshal(self.writeback)
	if err != nil {
		return fmt.Errorf("WriteFile to %v: %w", self.location, err)
	}

	if cb_err == WritebackUpdateLevel1 {
		// Write level 1 file.
		// Make sure the new file is only readable by root.
		err = ioutil.WriteFile(self.location, bytes, 0600)
		if err != nil {
			return fmt.Errorf("WriteFile to %v: %w", self.location, err)
		}
		return nil
	}

	if cb_err == WritebackUpdateLevel2 {
		// Write level 2 file.
		// Make sure the new file is only readable by root.
		err = ioutil.WriteFile(self.location, bytes, 0600)
		if err != nil {
			return fmt.Errorf("WriteFile to %v: %w", self.location, err)
		}
		return nil
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

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	writeback := &config_proto.Writeback{}

	data, err := ioutil.ReadFile(self.location)

	// Failing to read the file is not an error - the file may not
	// exist yet. In that case we just do not update our writeback.
	if err == nil {
		err = yaml.Unmarshal(data, writeback)
		if err == nil {
			logger.Debug("Writeback Manager: Loading writeback from %v", self.location)
			self.writeback = writeback
		}

		// If the install time in the writeback is not set, we update
		// it to now as it is the best guess of the install time.
		if self.writeback.InstallTime == 0 {
			self.writeback.InstallTime = uint64(utils.GetTime().Now().Unix())
			self.dirty = true
		}
		return nil
	}
	logger.Debug("Writeback Manager: <red>Unable to load writeback from %v</> Resetting file.", self.location)
	return nil
}

func NewWritebackManager(
	config_obj *config_proto.Config,
	location string) *WritebackManager {
	return &WritebackManager{
		config_obj: config_obj,
		location:   location,
		writeback:  &config_proto.Writeback{},
	}
}
