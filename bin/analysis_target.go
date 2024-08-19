package main

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/remapping"
)

var (
	remapping_flag = app.Flag(
		"remap", "A remapping configuration file for dead disk analysis.").String()
)

func applyRemapping(config_obj *config_proto.Config) error {
	if remapping_flag == nil || *remapping_flag == "" {
		return nil
	}

	data, err := ioutil.ReadFile(*remapping_flag)
	if err != nil {
		fmt.Printf("*** %v\n", err)
		return err
	}

	remapping_config := []*config_proto.RemappingConfig{}
	err = utils.YamlUnmarshalStrict(data, remapping_config)
	if err != nil {
		// It might be a regular config file
		full_config := &config_proto.Config{}
		err := utils.YamlUnmarshalStrict(data, full_config)
		if err != nil {
			return err
		}
		remapping_config = full_config.Remappings
	}
	if len(remapping_config) == 0 {
		return nil
	}

	Prelog("Applying remapping from %v", *remapping_flag)

	// Apply the remapping once to check for syntax errors so we can
	// fail early.
	device_manager := accessors.NewDefaultDeviceManager()

	// Create a scope without an ACL manager for verification. This is
	// too early in the startup process to initialize the proper
	// repository manager so we just make it up.
	scope := vql_subsystem.MakeScope()
	scope.AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	defer scope.Close()

	// Apply the remapping on this scope to catch any errors in the
	// remapping config.
	err = remapping.ApplyRemappingOnScope(
		context.Background(), config_obj, scope, scope, device_manager,
		ordereddict.NewDict(),
		remapping_config)
	if err != nil {
		return fmt.Errorf(
			"%v: Please check your config file's `remappings` setting", err)
	}

	// It is all good! Remapping accepted
	config_obj.Remappings = remapping_config

	return nil
}
