/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
)

var (
	// Keep track of inflight queries for shutdown. This wait group
	// belongs to the client's service manager. As we issue queries we
	// increment it and when queries are done we decrement it. The
	// service manager will wait for all inflight queries to exit
	// before exiting allowing the client to shut down in an orderly
	// fashion.
	mu          sync.Mutex
	service_wg  *sync.WaitGroup
	service_ctx context.Context = context.Background()

	GlobalEventTable *EventTable
)

type EventTable struct {
	mu sync.Mutex

	// Context for cancelling all inflight queries in this event
	// table.
	Ctx    context.Context
	cancel func()
	wg     sync.WaitGroup

	// The event table currently running
	Events []*actions_proto.VQLCollectorArgs

	// The version of this event table - we only update from the
	// server if the server's event table is newer.
	version uint64

	config_obj *config_proto.Config
}

// Determine if the current table is the same as the new set of
// queries. Returns true if the queries are the same and no change is
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
	self.mu.Lock()
	defer self.mu.Unlock()

	self.close()
}

// Actually close the table without lock
func (self *EventTable) close() {
	if self.config_obj != nil {
		logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
		logger.Info("Closing EventTable\n")
	}

	self.cancel()

	// Wait until the queries have completed.
	self.wg.Wait()

	// Clear the list of events we are tracking - we are an empty
	// event table right now - so further updates will restart the
	// queries again.
	self.Events = nil
}

func (self *EventTable) Version() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.version
}

func GlobalEventTableVersion() uint64 {
	return GlobalEventTable.Version()
}

func (self *EventTable) Update(
	config_obj *config_proto.Config,
	responder *responder.Responder,
	table *actions_proto.VQLEventTable) (*EventTable, error, bool) {

	self.mu.Lock()
	defer self.mu.Unlock()

	// Only update the event table if we need to.
	if table.Version <= self.version {
		return self, nil, false
	}

	// If the new update is identical to the old queries we wont
	// restart. This can happen e.g. if the server changes label
	// groups and recaculates the table version but the actual
	// queries dont end up changing.
	if self.equal(table.Event) {
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Info("Client event query update %v did not "+
			"change queries, skipping", table.Version)

		// Update the version only but keep queries the same.
		self.version = table.Version
		return self, nil, false
	}

	// Close the old table and wait for it to finish.
	self.close()

	// Reset the table with the new queries.
	GlobalEventTable = NewEventTable(config_obj, responder, table)
	return GlobalEventTable, nil, true /* changed */
}

type UpdateEventTable struct{}

func (self UpdateEventTable) Run(
	config_obj *config_proto.Config,
	ctx context.Context,
	responder *responder.Responder,
	arg *actions_proto.VQLEventTable) {

	// Make a new table if needed.
	table, err, changed := GlobalEventTable.Update(config_obj, responder, arg)
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

	// Start a new query for each event.
	action_obj := &VQLClientAction{}
	table.wg.Add(len(table.Events))
	service_wg.Add(len(table.Events))

	for _, event := range table.Events {
		query_responder := responder.Copy(table.Ctx)

		go func(event *actions_proto.VQLCollectorArgs) {
			defer table.wg.Done()
			defer service_wg.Done()

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

			// Start the query - if it is an event query this will
			// never complete until it is cancelled.
			action_obj.StartQuery(
				config_obj, table.Ctx, query_responder, event)
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

	// Read the existing writeback file - it is ok if it does not
	// exist yet.
	writeback, _ := config.GetWriteback(config_obj.Client)
	writeback.EventQueries = event_table

	return config.UpdateWriteback(config_obj.Client, writeback)
}

func NewEventTable(
	config_obj *config_proto.Config,
	responder *responder.Responder,
	table *actions_proto.VQLEventTable) *EventTable {

	sub_ctx, cancel := context.WithCancel(service_ctx)

	result := &EventTable{
		Events:     table.Event,
		version:    table.Version,
		Ctx:        sub_ctx,
		cancel:     cancel,
		config_obj: config_obj,
	}

	return result
}

// Called by the service manager to initialize the global event table.
func InitializeEventTable(
	// This is the context of the service - its lifetime represents
	// the lifetime of the entire application.
	ctx context.Context,
	config_obj *config_proto.Config,
	output_chan chan *crypto_proto.VeloMessage,
	wg *sync.WaitGroup) {

	mu.Lock()
	service_ctx = ctx
	service_wg = wg

	// Remove any old tables if needed.
	if GlobalEventTable != nil {
		GlobalEventTable.Close()
	}

	// Create an empty table
	GlobalEventTable = NewEventTable(
		config_obj,
		responder.NewResponder(ctx,
			config_obj, &crypto_proto.VeloMessage{}, output_chan),
		&actions_proto.VQLEventTable{})

	// When the context is finished, tear down the event table.
	go func(table *EventTable, ctx context.Context) {
		<-ctx.Done()
		table.Close()
	}(GlobalEventTable, service_ctx)

	mu.Unlock()
}

func init() {
	ctx, cancel := context.WithCancel(context.Background())
	GlobalEventTable = &EventTable{
		Ctx:    ctx,
		cancel: cancel,
	}
}
