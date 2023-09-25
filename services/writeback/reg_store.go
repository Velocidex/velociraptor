// +build windows

// A writeback store that supports the windows registry.

package writeback

import (
	"fmt"
	"strings"

	"github.com/Velocidex/yaml/v2"
	"golang.org/x/sys/windows/registry"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

type RegistryWritebackStore struct {
	config_obj *config_proto.Config

	// The registry key to hold everything.
	location string
}

func (self *RegistryWritebackStore) getKey() (registry.Key, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, self.location,
		registry.SET_VALUE|registry.READ)
	if err != nil {
		k, _, err = registry.CreateKey(registry.LOCAL_MACHINE, self.location,
			registry.SET_VALUE|registry.READ)
		if err != nil {
			return 0, err
		}
	}

	return k, nil
}

// The L1 details are written separately
func (self *RegistryWritebackStore) WriteL1(wb *config_proto.Writeback) error {
	k, err := self.getKey()
	if err != nil {
		return err
	}
	defer k.Close()

	err = k.SetDWordValue("InstallTime", uint32(wb.InstallTime))
	if err != nil {
		return err
	}

	err = k.SetStringValue("PrivateKey", wb.PrivateKey)
	if err != nil {
		return err
	}

	err = k.SetStringValue("ClientId", wb.ClientId)
	if err != nil {
		return err
	}

	return nil
}

// The Level2 details are written as a single REG_BINARY blob
func (self *RegistryWritebackStore) WriteL2(wb *config_proto.Writeback) error {
	bytes, err := yaml.Marshal(wb)
	if err != nil {
		return fmt.Errorf("Writeback WriteL2 to %v: %w", self.location, err)
	}

	k, err := self.getKey()
	if err != nil {
		return err
	}
	defer k.Close()

	err = k.SetStringValue("WriteBack", string(bytes))
	if err != nil {
		return err
	}

	return nil
}

func (self *RegistryWritebackStore) Load() *config_proto.Writeback {
	wb := &config_proto.Writeback{}

	k, err := self.getKey()
	if err != nil {
		return wb
	}
	defer k.Close()

	serialized, _, err := k.GetStringValue("WriteBack")
	if err == nil {
		_ = yaml.Unmarshal([]byte(serialized), wb)
	}

	// Now fill in the L1 data if available.
	_, install, err := k.GetIntegerValue("InstallTime")
	if err == nil {
		wb.InstallTime = uint64(install)
	}

	private_key, _, err := k.GetStringValue("PrivateKey")
	if err == nil {
		wb.PrivateKey = private_key
	}

	client_id, _, err := k.GetStringValue("ClientId")
	if err == nil {
		wb.ClientId = client_id
	}

	if wb.InstallTime == 0 {
		wb.InstallTime = uint64(utils.GetTime().Now().Unix())
		self.WriteL1(wb)
	}

	return wb
}

func GetFileWritebackStore(config_obj *config_proto.Config) WritebackStorer {
	location, _ := WritebackLocation(config_obj)

	if strings.HasPrefix(location, "HKLM\\") {
		return &RegistryWritebackStore{
			config_obj: config_obj,
			location:   strings.TrimPrefix(location, "HKLM\\"),
		}
	}

	return &FileWritebackStore{
		config_obj:  config_obj,
		location:    location,
		l2_location: location + config_obj.Client.Level2WritebackSuffix,
	}
}
