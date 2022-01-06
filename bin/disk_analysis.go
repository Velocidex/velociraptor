package main

import (
	"errors"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func verifyDiskAnalysisSettings(config_obj *config_proto.Config) error {
	if config_obj.Device != "" && config_obj.DeviceAccessor == "" {
		return errors.New("cannot use `device' flag without `device_accessor' flag")
	}

	return nil
}
