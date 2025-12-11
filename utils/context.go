package utils

import config_proto "www.velocidex.com/golang/velociraptor/config/proto"

// True when we are running on the client.
func RunningOnClient(config_obj *config_proto.Config) bool {
	// Pure clients do not have any frontend configs.
	if config_obj.Frontend == nil || config_obj.Datastore == nil {
		return true
	}

	// Clients also run the event table service.
	if config_obj.Services != nil {
		if config_obj.Services.ClientEventTable {
			return true
		}

		// Server only run the hunt dispatcher.
		if config_obj.Services.HuntDispatcher {
			return false
		}
	}

	return false
}
