package services

// The Velociraptor server maintains a table of event queries it runs
// on startup. This service manages this table. It provides methods
// for the Velociraptor administrator to update the table for the
// server.

import (
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

var (
	server_event_mu    sync.Mutex
	ServerEventManager serverEvent
)

func RegisterServerEventManager(manager serverEvent) {
	server_event_mu.Lock()
	defer server_event_mu.Unlock()

	ServerEventManager = manager
}

func GetServerEventManager() serverEvent {
	server_event_mu.Lock()
	defer server_event_mu.Unlock()

	return ServerEventManager
}

type serverEvent interface {
	// Update the server's event table.
	Update(config_obj *config_proto.Config,
		arg *flows_proto.ArtifactCollectorArgs) error

	// Close the event manager and cleanup.
	Close()
}
