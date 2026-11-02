package main

import (
	"context"
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/frontend"
)

// Minion mode acts as a minion worker:
// 1. Server specific plugins are redirected to the server
// 2. The filestore accessor is redirected over the RPC connection
func installMinionMode(
	ctx context.Context,
	config_obj *config_proto.Config) error {
	err := frontend.InitializeMinionVQL(config_obj)
	if err != nil {
		return err
	}

	// Minion mode requires superuser credentials.
	if config_obj.ApiConfig != nil {
		config_obj.ApiConfig.Name = ""
	}

	if config_obj.Frontend == nil || config_obj.Datastore == nil {
		return fmt.Errorf("Minion mode requires a full server config file")
	}

	// Enable the remote filestore
	config_obj.Datastore.Implementation = "RemoteRPCFileStore"

	config_obj.Services = services.GenericToolServices()
	config_obj.Services.FrontendServer = true
	config_obj.Services.InventoryService = true
	config_obj.Services.ServerArtifacts = true
	config_obj.Services.IndexServer = true
	config_obj.Frontend.IsMinion = true

	return nil
}
