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
// table is kept in sync with the server by comparing the table's
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

	// This will be closed to signal that we need to abort the
	// current event queries.
	Done chan bool
	wg   sync.WaitGroup

	// Keep track of inflight queries for shutdown. This wait
	// group belongs to the client's service manager. As we issue
	// queries we increment it and when queries are done we
	// decrement it. The service manager will wait for this before
	// exiting allowing the client to shut down in an orderly
	// fashion.
	service_wg *sync.WaitGroup
}

// Determine if the current table is the same as the new set of
// queries. Returns true if the queries are the same and not change is
// needed.
func (self *EventTable) equal(events []*actions_proto.VQLCollectorArgs) bool {
	if len(events) != len(self.Events) {
		return false
	}

	for i := range self.Events {
		lhs := self.Events[i]
		rhs := events[i]

		if len(lhs.Query) != len(rhs.Query) {
			return false
		}

		for j := range lhs.Query {
			if !proto.Equal(lhs.Query[j], rhs.Query[j]) {
				return false
			}
		}

		if len(lhs.Env) != len(rhs.Env) {
			return false
		}

		for j := range lhs.Env {
			if !proto.Equal(lhs.Env[j], rhs.Env[j]) {
				return false
			}
		}
	}
	return true
}

// Teardown all the current quries. Blocks until they all shut down.
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
	table *actions_proto.VQLEventTable) (*EventTable, error, bool) {

	mu.Lock()
	defer mu.Unlock()

	// Only update the event table if we need to.
	if table.Version <= GlobalEventTable.version {
		return GlobalEventTable, nil, false
	}

	// If the new update is identical to the old queries we wont
	// restart. This can happen e.g. if the server changes label
	// groups and recaculates the table version but the actual
	// queries dont end up changing.
	if GlobalEventTable.equal(table.Event) {
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Info("Client event query update %v did not "+
			"change queries, skipping", table.Version)

		// Update the version only but keep queries the same.
		GlobalEventTable.version = table.Version
		return GlobalEventTable, nil, false
	}

	// Close the old table.
	if GlobalEventTable.Done != nil {
		GlobalEventTable.Close()
	}

	// Reset the table.
	GlobalEventTable.Events = table.Event
	GlobalEventTable.version = table.Version
	GlobalEventTable.Done = make(chan bool)
	GlobalEventTable.config_obj = config_obj
	GlobalEventTable.service_wg = &sync.WaitGroup{}

	return GlobalEventTable, nil, true /* changed */
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
	table, err, changed := update(config_obj, responder, arg)
	if err != nil {
		responder.RaiseError(ctx, fmt.Sprintf(
			"Error updating global event table: %v", err))
		return
	}

	// No change required, skip it.
	if !changed {
		// We still need to write the new version
		err = update_writeback(config_obj, arg)
		if err != nil {
			responder.RaiseError(ctx, fmt.Sprintf(
				"Unable to write events to writeback: %v", err))
		} else {
			responder.Return(ctx)
		}
		return
	}

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// Make a context for the VQL query.
	new_ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context when the cancel channel is closed.
	go func() {
		mu.Lock()
		done := table.Done
		mu.Unlock()

		<-done
		logger.Info("UpdateEventTable: Closing all contexts")
		cancel()
	}()

	// Start a new query for each event.
	action_obj := &VQLClientAction{}
	table.wg.Add(len(table.Events))
	table.service_wg.Add(len(table.Events))

	for _, event := range table.Events {
		query_responder := responder.Copy()

		go func(event *actions_proto.VQLCollectorArgs) {
			defer table.wg.Done()
			defer table.service_wg.Done()

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
		}(proto.Clone(event).(*actions_proto.VQLCollectorArgs))
	}

	err = update_writeback(config_obj, arg)
	if err != nil {
		responder.RaiseError(ctx, fmt.Sprintf(
			"Unable to write events to writeback: %v", err))
		return
	}

	responder.Return(ctx)
}

func update_writeback(
	config_obj *config_proto.Config,
	event_table *actions_proto.VQLEventTable) error {

	// Store the event table in the Writeback file.
	config_copy := proto.Clone(config_obj).(*config_proto.Config)
	event_copy := proto.Clone(event_table).(*actions_proto.VQLEventTable)
	config_copy.Writeback.EventQueries = event_copy

	return config.UpdateWriteback(config_copy)
}

func InitializeEventTable(ctx context.Context, service_wg *sync.WaitGroup) {
	mu.Lock()
	GlobalEventTable = &EventTable{
		service_wg: service_wg,
	}
	mu.Unlock()

	// When the context is finished, tear down the event table.
	go func() {
		<-ctx.Done()

		mu.Lock()
		if GlobalEventTable.Done != nil {
			close(GlobalEventTable.Done)
		}
		mu.Unlock()
	}()

}
