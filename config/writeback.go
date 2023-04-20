// Manage the writeback file.

// The writeback file is used to store client side state. This file
// manages it as part of the client configuration.
package config

import (
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/Velocidex/yaml/v2"
	"github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *Loader) loadWriteback(config_obj *config_proto.Config) error {
	filename, err := WritebackLocation(config_obj.Client)
	if err != nil {
		return err
	}
	self.Log("Loading writeback from %v", filename)

	_, err = GetWriteback(config_obj.Client)
	if err != nil {
		// writeback file is invalid... Log an error and reset it
		// otherwise the client will fail to start and break.
		if err != nil {
			self.Log("Writeback file is corrupt - resetting: %v", err)
		}
	}

	return nil
}

var (
	mu       sync.Mutex
	NoUpdate = errors.New("No update")
)

func MutateWriteback(
	config_obj *config_proto.ClientConfig,
	cb func(wb *config_proto.Writeback) error) error {

	if config_obj == nil {
		return nil
	}

	mu.Lock()
	defer mu.Unlock()

	wb, err := GetWriteback(config_obj)
	if err != nil {
		return err
	}

	err = cb(wb)
	if err == NoUpdate {
		return nil
	}

	if err != nil {
		return err
	}
	return UpdateWriteback(config_obj, wb)
}

func GetWriteback(config_obj *config_proto.ClientConfig) (
	*config_proto.Writeback, error) {
	result := &config_proto.Writeback{}

	filename, err := WritebackLocation(config_obj)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(filename)

	// Failing to read the file is not an error - the file may not
	// exist yet.
	if err == nil {
		err = yaml.Unmarshal(data, result)
		if err != nil {
			return result, nil
		}

		// If the install time in the writeback is not set, we update
		// it to now as it is the best guess of the install time.
		if result.InstallTime == 0 {
			result.InstallTime = uint64(utils.GetTime().Now().Unix())
			// Update the writeback with the current time as install.
			return result, UpdateWriteback(config_obj, result)
		}

		return result, nil
	}

	return result, nil
}

// Update the client's writeback file.
func UpdateWriteback(
	config_obj *config_proto.ClientConfig,
	writeback *config_proto.Writeback) error {
	if config_obj == nil {
		return errors.New("No Client config")
	}

	location, err := WritebackLocation(config_obj)
	if err != nil {
		return err
	}

	if writeback.InstallTime == 0 {
		writeback.InstallTime = uint64(utils.GetTime().Now().Unix())
	}

	bytes, err := yaml.Marshal(writeback)
	if err != nil {
		return errors.Wrap(err, 0)
	}

	// Make sure the new file is only readable by root.
	err = ioutil.WriteFile(location, bytes, 0600)
	if err != nil {
		return fmt.Errorf("WriteFile to %v: %w", location, err)
	}

	return nil
}
