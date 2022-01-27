package main

import (
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/remapping"
)

func applyAnalysisTarget(config_obj *config_proto.Config) error {
	_, err := remapping.GetDeviceManager(config_obj)
	if err != nil {
		return fmt.Errorf(
			"%v: Please check your config file's `remappings` setting", err)
	}
	return nil
}
