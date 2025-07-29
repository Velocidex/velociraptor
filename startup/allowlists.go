package startup

import (
	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// Potentially restrict server functionality.
func MaybeEnforceAllowLists(config_obj *config_proto.Config) error {
	if config_obj.Security == nil {
		return nil
	}

	if len(config_obj.Security.AllowedPlugins) > 0 {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Restricting VQL plugins to set %v and functions to set %v\n",
			config_obj.Security.AllowedPlugins, config_obj.Security.AllowedFunctions)
	}

	if len(config_obj.Security.DeniedPlugins) > 0 {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Removing VQL plugins to set %v and functions to set %v\n",
			config_obj.Security.DeniedPlugins, config_obj.Security.DeniedPlugins)
	}

	err := vql_subsystem.EnforceVQLAllowList(
		config_obj.Security.AllowedPlugins,
		config_obj.Security.AllowedFunctions,
		config_obj.Security.DeniedPlugins,
		config_obj.Security.DeniedFunctions)
	if err != nil {
		return err
	}

	if len(config_obj.Security.AllowedAccessors) > 0 ||
		len(config_obj.Security.DeniedAccessors) > 0 {
		err = accessors.EnforceAccessorAllowList(
			config_obj.Security.AllowedAccessors,
			config_obj.Security.DeniedAccessors,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
