package services

/*
   The Velociraptor client maintains a table of event queries it runs
   on startup. This service manages this table. It provides methods
   for the Velociraptor administrator to update the table for this
   client, and methods for the client to resync its table.

   Clients receive an event table specific for them - depending on
   their label assignment. Callers can receive the correct update
   message for the client by calling
   GetClientUpdateEventTableMessage().

   It is only necessary to update the client if its version is behind
   what it should be. Callers can check if the cliet's event table is
   current by calling CheckClientEventsVersion(). This is a very fast
   option and so it is appropriate to call it from the critical path.

*/

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

func ClientEventManager(config_obj *config_proto.Config) (ClientEventTable, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).ClientEventManager()
}

type ClientEventTable interface {
	// Get the version of the client event table for this
	// client. If the client's version is lower then we resync the
	// client's event table.
	CheckClientEventsVersion(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string, client_version uint64) bool

	// Get the message to send to the client in order to force it
	// to update.
	GetClientUpdateEventTableMessage(
		ctx context.Context,
		config_obj *config_proto.Config,
		client_id string) *crypto_proto.VeloMessage

	// Get the specs that will be applied on a client depending on
	// label membership
	GetClientSpec(ctx context.Context, config_obj *config_proto.Config,
		client_id string) []*flows_proto.ArtifactSpec

	// Get the full client monitoring table.
	GetClientMonitoringState() *flows_proto.ClientEventTable

	// Set the client monitoring table.
	SetClientMonitoringState(
		ctx context.Context,
		config_obj *config_proto.Config,
		principal string,
		state *flows_proto.ClientEventTable) error

	ListAvailableEventResults(
		ctx context.Context,
		in *api_proto.ListAvailableEventResultsRequest) (
		*api_proto.ListAvailableEventResultsResponse, error)
}
