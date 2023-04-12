package services

// The Velociraptor server maintains a table of event queries it runs
// on startup. This service manages this table. It provides methods
// for the Velociraptor administrator to update the table for the
// server.

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

func GetServerEventManager(config_obj *config_proto.Config) (ServerEventManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).ServerEventManager()
}

type ServerEventManager interface {
	// Update the server's event table.
	Update(ctx context.Context,
		config_obj *config_proto.Config,
		principal string,
		arg *flows_proto.ArtifactCollectorArgs) error

	Get() *flows_proto.ArtifactCollectorArgs

	// Close the event manager and cleanup.
	Close()
}
