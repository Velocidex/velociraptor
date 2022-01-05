package main

import (
	"errors"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func applyDiskAnalysisSettings(config_obj *config_proto.Config) error {
	config_obj.AnalysisTarget = *analysis_target_flag
	config_obj.Device = *device_flag
	config_obj.DeviceAccessor = *device_accessor_flag

	if *device_flag != "" && *device_accessor_flag == "" {
		return errors.New("cannot use `device' flag without `device_accessor' flag")
	}

	return nil
}
