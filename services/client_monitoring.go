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
	"sync"

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
	CheckClientEventsVersion(client_id string, client_version uint64) bool

	// Get the message to send to the client in order to force it
	// to update.
	GetClientUpdateEventTableMessage(client_id string) *crypto_proto.GrrMessage

	// Get the full client monitoring table.
	GetClientMonitoringState() *flows_proto.ClientEventTable

	// Set the client monitoring table.
	SetClientMonitoringState(state *flows_proto.ClientEventTable) error
}
