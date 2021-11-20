package main

import config_proto "www.velocidex.com/golang/velociraptor/config/proto"

func applyMinionRole(config_obj *config_proto.Config) error {
	if config_obj.Frontend != nil {
		config_obj.Frontend.IsMinion = *frontend_cmd_minion
	}
	return nil
}
