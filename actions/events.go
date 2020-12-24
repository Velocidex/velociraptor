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
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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

	config_obj *config_proto.Config

	// This will be closed to signal we need to abort the current
	// event queries.
	Done chan bool
	wg   sync.WaitGroup
}

func (self *EventTable) Close() {
	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	logger.Info("Closing EventTable\n")

	close(self.Done)
	self.wg.Wait()
}

func GlobalEventTableVersion() uint64 {
	mu.Lock()
	defer mu.Unlock()

	return GlobalEventTable.version
}

func update(
	config_obj *config_proto.Config,
	responder *responder.Responder,
	table *actions_proto.VQLEventTable) (*EventTable, error) {

	mu.Lock()
	defer mu.Unlock()

	// Only update the event table if we need to.
	if table.Version <= GlobalEventTable.version {
		return GlobalEventTable, nil
	}

	// Close the old table.
	if GlobalEventTable.Done != nil {
		GlobalEventTable.Close()
	}

	// Make a new table.
	GlobalEventTable = NewEventTable(
		config_obj, responder, table)

	return GlobalEventTable, nil
}

func NewEventTable(
	config_obj *config_proto.Config,
	responder *responder.Responder,
	table *actions_proto.VQLEventTable) *EventTable {
	result := &EventTable{
		Events:     table.Event,
		version:    table.Version,
		Done:       make(chan bool),
		config_obj: config_obj,
	}

	return result
}

type UpdateEventTable struct{}

func (self UpdateEventTable) Run(
	config_obj *config_proto.Config,
	ctx context.Context,
	responder *responder.Responder,
	arg *actions_proto.VQLEventTable) {

	// Make a new table.
	table, err := update(config_obj, responder, arg)
	if err != nil {
		responder.Log(ctx, "Error updating global event table: %v", err)
	}

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// Make a context for the VQL query.
	new_ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context when the cancel channel is closed.
	go func() {
		<-table.Done
		logger.Info("UpdateEventTable: Closing all contexts")
		cancel()
	}()

	// Start a new query for each event.
	action_obj := &VQLClientAction{}
	table.wg.Add(len(table.Events))

	for _, event := range table.Events {
		query_responder := responder.Copy()

		go func(event *actions_proto.VQLCollectorArgs) {
			defer table.wg.Done()

			// Name of the query we are running.
			name := ""
			for _, q := range event.Query {
				if q.Name != "" {
					name = q.Name
				}
			}

			if name != "" {
				logger.Info("<green>Starting</> monitoring query %s", name)
			}
			query_responder.Artifact = name

			// Event tables never time out
			if event.Timeout == 0 {
				event.Timeout = 99999999
			}

			// Dont heartbeat too often for event queries
			// - the log generates un-neccesary traffic.
			if event.Heartbeat == 0 {
				event.Heartbeat = 300 // 5 minutes
			}

			action_obj.StartQuery(
				config_obj, new_ctx, query_responder, event)
			if name != "" {
				logger.Info("Finished monitoring query %s", name)
			}
		}(event)
	}

	// Store the event table in the Writeback file.
	config_copy := proto.Clone(config_obj).(*config_proto.Config)
	event_copy := proto.Clone(arg).(*actions_proto.VQLEventTable)
	config_copy.Writeback.EventQueries = event_copy
	err = config.UpdateWriteback(config_copy)
	if err != nil {
		responder.RaiseError(ctx, fmt.Sprintf(
			"Unable to write events to writeback: %v", err))
	}

	responder.Return(ctx)
}
