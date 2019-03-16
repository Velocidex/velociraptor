/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
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
	version uint64

	// This will be closed to signal we need to abort the current
	// event queries.
	Done chan bool
}

func GlobalEventTableVersion() uint64 {
	mu.Lock()
	defer mu.Unlock()

	return GlobalEventTable.version
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
		version: table.Version,
		Done:    make(chan bool),
	}

	return result
}
