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

type EventTable struct {
	mu sync.Mutex

	// Context for cancelling all inflight queries in this event
	// table.
	Ctx    context.Context
	cancel func()
	wg     *sync.WaitGroup

	// The event table currently running
	Events []*actions_proto.VQLCollectorArgs

	// The version of this event table - we only update from the
	// server if the server's event table is newer.
	version uint64

	config_obj *config_proto.Config

	monitoring_manager *responder.MonitoringManager
}

// Determine if the current table is the same as the new set of
// queries. Returns true if the queries are the same and no change is
// needed. NOTE: Assumes the order of queries and Env variables is
// deterministic and consistent.
func (self *EventTable) Equal(events []*actions_proto.VQLCollectorArgs) bool {
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
	self.version = 0
}

func (self *EventTable) Version() uint64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.version
}

func (self *EventTable) Update(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	output_chan chan *crypto_proto.VeloMessage,
	table *actions_proto.VQLEventTable) (error, bool) {

	self.mu.Lock()
	defer self.mu.Unlock()

	// Only update the event table if we need to.
	if table.Version <= self.version {
		return nil, false
	}

	// If the new update is identical to the old queries we wont
	// restart. This can happen e.g. if the server changes label
	// groups and recaculates the table version but the actual
	// queries dont end up changing.
	if self.Equal(table.Event) {
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Info("Client event query update %v did not "+
			"change queries, skipping", table.Version)

		// Update the version only but keep queries the same.
		self.version = table.Version
		return nil, false
	}

	// Close the old table and wait for it to finish.
	self.close()

	// Reset the event table and start from scratch.
	self.Events = nil

	// Make a copy of the events so we can own them.
	for _, e := range table.Event {
		self.Events = append(self.Events,
			proto.Clone(e).(*actions_proto.VQLCollectorArgs))
	}

	self.version = table.Version
	self.wg = &sync.WaitGroup{}
	self.Ctx, self.cancel = context.WithCancel(ctx)

	return nil, true /* changed */
}

func (self *EventTable) StartQueries(
	ctx context.Context,
	config_obj *config_proto.Config,
	output_chan chan *crypto_proto.VeloMessage) {

	self.mu.Lock()
	defer self.mu.Unlock()

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	// Start a new query for each event.
	action_obj := &VQLClientAction{}
	for _, event := range self.Events {

		// Name of the query we are running. There must be at least
		// one query with a name.
		artifact_name := GetQueryName(event.Query)
		if artifact_name == "" {
			continue
		}

		logger.Info("<green>Starting</> monitoring query %s", artifact_name)
		query_responder := responder.NewMonitoringResponder(
			ctx, config_obj, self.monitoring_manager,
			output_chan, artifact_name)

		self.wg.Add(1)
		go func(event *actions_proto.VQLCollectorArgs) {
			defer self.wg.Done()

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
				config_obj, self.Ctx, query_responder, event)
			if artifact_name != "" {
				logger.Info("Finished monitoring query %s", artifact_name)
			}
		}(proto.Clone(event).(*actions_proto.VQLCollectorArgs))
	}
}

func (self *EventTable) StartFromWriteback(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	output_chan chan *crypto_proto.VeloMessage) {

	// Get the event table from the writeback if possible.
	event_table := &actions_proto.VQLEventTable{}

	writeback, err := config.GetWriteback(config_obj.Client)
	if err == nil && writeback.EventQueries != nil {
		event_table = writeback.EventQueries
		self.UpdateEventTable(ctx, wg, config_obj, output_chan, event_table)
	}
}

func (self *EventTable) UpdateEventTable(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	output_chan chan *crypto_proto.VeloMessage,
	update_table *actions_proto.VQLEventTable) {

	// Make a new table if needed.
	err, changed := self.Update(
		ctx, wg, config_obj, output_chan, update_table)
	if err != nil {
		responder.MakeErrorResponse(
			output_chan, "F.Monitoring", fmt.Sprintf(
				"Error updating global event table: %v", err))
		return
	}

	// No change required, skip it.
	if !changed {
		// We still need to write the new version
		err = update_writeback(config_obj, update_table)
		if err != nil {
			responder.MakeErrorResponse(output_chan, "F.Monitoring",
				fmt.Sprintf("Unable to write events to writeback: %v", err))
		}
		return
	}

	// Kick off the queries
	self.StartQueries(ctx, config_obj, output_chan)

	err = update_writeback(config_obj, update_table)
	if err != nil {
		responder.MakeErrorResponse(output_chan, "F.Monitoring",
			fmt.Sprintf("Unable to write events to writeback: %v", err))
		return
	}
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
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) *EventTable {

	sub_ctx, cancel := context.WithCancel(ctx)
	self := &EventTable{
		Ctx:    sub_ctx,
		cancel: cancel,

		// Used to wait for close()
		wg:                 &sync.WaitGroup{},
		config_obj:         config_obj,
		monitoring_manager: responder.NewMonitoringManager(ctx),
	}

	return self
}
