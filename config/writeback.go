// Manage the writeback file.

// The writeback file is used to store client side state. This file
// manages it as part of the client configuration.
package config

import (
	"io/ioutil"

	"github.com/Velocidex/yaml/v2"
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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
		return result, yaml.Unmarshal(data, result)
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

	bytes, err := yaml.Marshal(writeback)
	if err != nil {
		return errors.WithStack(err)
	}

	// Make sure the new file is only readable by root.
	err = ioutil.WriteFile(location, bytes, 0600)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
