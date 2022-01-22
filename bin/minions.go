package main

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/frontend"
)

func applyMinionRole(config_obj *config_proto.Config) error {
	if config_obj.Frontend == nil {
		return nil
	}

	// Is this a minion? record it in the config file.
	config_obj.Frontend.IsMinion = *frontend_cmd_minion

	if *frontend_cmd_node != "" {
		// Mutate the config file to select the correct node
		// config. From here on config_obj.Frontend refers to the
		// correct node.
		err := frontend.SelectFrontend(*frontend_cmd_node, config_obj)
		if err != nil {
			return err
		}
	}

	// Minions need to log to a different place so they do not
	// overwrite logs from the master.
	if config_obj.Frontend.IsMinion {
		logging.SetNodeName(services.GetNodeName(config_obj.Frontend))
	}

	return nil
}
