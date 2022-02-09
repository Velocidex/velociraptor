package main

import (
	"context"
	"fmt"

	"github.com/Velocidex/yaml/v2"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

var (
	remapping_flag = app.Flag(
		"remap", "A remapping configuration for dead disk analysis.").Strings()
)

func applyAnalysisTarget(config_obj *config_proto.Config) error {
	for _, remap := range *remapping_flag {
		remapping_config := &config_proto.RemappingConfig{}
		err := yaml.Unmarshal([]byte(remap), remapping_config)
		if err != nil {
			return err
		}
		logging.Prelog("Applying remapping %v", remapping_config)

		config_obj.Remappings = append(config_obj.Remappings, remapping_config)
	}

	if len(config_obj.Remappings) == 0 {
		return nil
	}

	// Apply the remapping once to check for syntax errors so we can
	// fail early.
	device_manager := accessors.NewDefaultDeviceManager()
	err := accessors.ApplyRemappingOnScope(
		context.Background(), device_manager, config_obj.Remappings)
	if err != nil {
		return fmt.Errorf(
			"%v: Please check your config file's `remappings` setting", err)
	}
	return nil
}
