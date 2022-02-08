package main

import (
	"fmt"

	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func applyAnalysisTarget(config_obj *config_proto.Config) error {
	_, err := accessors.GetDeviceManager(config_obj)
	if err != nil {
		return fmt.Errorf(
			"%v: Please check your config file's `remappings` setting", err)
	}
	return nil
}
