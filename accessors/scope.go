package accessors

import (
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func GetDeviceManager(config_obj *config_proto.Config) (DeviceManager, error) {
	if config_obj.Remappings == nil {
		return GlobalDeviceManager, nil
	}

	// Build the device manager according to the remapping configuration.
	manager := GlobalDeviceManager.Copy()
	for _, remapping := range config_obj.Remappings {
		switch remapping.Type {
		case "mount":
			err := InstallMountPoint(manager, remapping)
			if err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf(
				"Invalid remapping directive: %v", remapping.Type)
		}
	}

	return manager, nil
}
