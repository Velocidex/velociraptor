// Manage events listener.

// Events are long lived VQL queries which return their results to the
// event handler. The event table is refreshed periodically by the server.

package events

import (
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

var (
	GlobalEventTable = &EventTable{}
	mu               sync.Mutex
)

type EventTable struct {
	Events  []*actions_proto.VQLCollectorArgs
	Version uint64

	// This will be closed to signal we need to abort the current
	// event queries.
	Done chan bool
}

func Update(
	responder *responder.Responder,
	table *actions_proto.VQLEventTable) (*EventTable, error) {

	mu.Lock()
	defer mu.Unlock()

	// Close the old table.
	if GlobalEventTable.Done != nil {
		close(GlobalEventTable.Done)
	}

	// Make a new table.
	GlobalEventTable = NewEventTable(responder, table)

	return GlobalEventTable, nil
}

func NewEventTable(
	responder *responder.Responder,
	table *actions_proto.VQLEventTable) *EventTable {
	result := &EventTable{
		Events:  table.Event,
		Version: table.Version,
		Done:    make(chan bool),
	}

	return result
}
