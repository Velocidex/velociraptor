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

// Client Events are long lived VQL queries which stream their results
// to the event handler on the server. Clients maintain a global event
// table internally containing a set of event queries. The client's
// table is kept in sync with the server by compaing the table's
// version on each packet sent. If the server's event table is higher
// than the client's the server will refresh the client's table using
// the UpdateEventTable() action.

package actions

import (
	"context"
	"sync"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
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

func update(
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

type UpdateEventTable struct{}

func (self *UpdateEventTable) Run(
	config *api_proto.Config,
	ctx context.Context,
	msg *crypto_proto.GrrMessage,
	output chan<- *crypto_proto.GrrMessage) {
	responder := responder.NewResponder(msg, output)
	arg, pres := responder.GetArgs().(*actions_proto.VQLEventTable)
	if !pres {
		responder.RaiseError("Request should be of type VQLEventTable")
		return
	}

	// Make a new table.
	table, err := update(responder, arg)
	if err != nil {
		responder.Log("Error updating global event table: %v", err)
	}

	// Make a context for the VQL query.
	new_ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context when the cancel channel is closed.
	go func() {
		<-table.Done
		cancel()
	}()

	logger := logging.GetLogger(config, &logging.ClientComponent)

	// Start a new query for each event.
	action_obj := &VQLClientAction{}
	var wg sync.WaitGroup
	wg.Add(len(table.Events))

	for _, event := range table.Events {
		go func(event *actions_proto.VQLCollectorArgs) {
			defer wg.Done()

			name := ""
			for _, q := range event.Query {
				if q.Name != "" {
					name = q.Name
				}
			}

			logger.Info("Starting %s", name)
			action_obj.StartQuery(
				config, new_ctx, responder, event)

			logger.Info("Finished %s", name)
		}(event)
	}

	// Return an OK status. This is needed to make sure the
	// request is de-queued.
	responder.Return()

	// Wait here for all queries to finish - this forces the
	// output channel to be open and allows us to write results to
	// the server.
	wg.Wait()
}
