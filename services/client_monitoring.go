package services

// The Velociraptor client maintains a table of event queries it runs
// on startup. This service manages this table. It provides methods
// for the Velociraptor administrator to update the table for this
// client, and methods for the client to resync its table.

import (
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

var (
	client_manager_mu    sync.Mutex
	client_event_manager ClientEventTable
)

func ClientEventManager() ClientEventTable {
	client_manager_mu.Lock()
	defer client_manager_mu.Unlock()

	return client_event_manager
}

func RegisterClientEventManager(manager ClientEventTable) {
	client_manager_mu.Lock()
	defer client_manager_mu.Unlock()

	client_event_manager = manager
}

type ClientEventTable interface {
	// Get the version of the client event table for this
	// client. If the client's version is lower then we resync the
	// client's event table.
	GetClientEventsVersion(client_id string) uint64

	// Get the message to send to the client in order to force it
	// to update.
	GetClientUpdateEventTableMessage() *crypto_proto.GrrMessage

	// Update the event table for this client.
	UpdateClientEventTable(config_obj *config_proto.Config,
		args *flows_proto.ArtifactCollectorArgs, label string) error
}
