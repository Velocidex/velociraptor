package startup

import (
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// Potentially restrict server functionality.
func MaybeEnforceAllowLists(config_obj *config_proto.Config) error {
	if config_obj.Defaults == nil {
		return nil
	}

	if len(config_obj.Defaults.AllowedPlugins) > 0 {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Restricting VQL plugins to set %v and functions to set %v\n",
			config_obj.Defaults.AllowedPlugins, config_obj.Defaults.AllowedFunctions)
	}

	if len(config_obj.Defaults.DeniedPlugins) > 0 {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Removing VQL plugins to set %v and functions to set %v\n",
			config_obj.Defaults.DeniedPlugins, config_obj.Defaults.DeniedPlugins)
	}

	err := vql_subsystem.EnforceVQLAllowList(
		config_obj.Defaults.AllowedPlugins,
		config_obj.Defaults.AllowedFunctions,
		config_obj.Defaults.DeniedPlugins,
		config_obj.Defaults.DeniedFunctions)
	if err != nil {
		return err
	}

	if len(config_obj.Defaults.AllowedAccessors) > 0 {
		err = accessors.EnforceAccessorAllowList(
			config_obj.Defaults.AllowedAccessors)
		if err != nil {
			return err
		}
	}

	return nil
}
