/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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
	"time"

	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
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
	// groups and recalculates the table version but the actual
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

// Make a copy of the event table and appand any config enforced
// additional event queries.
func (self *EventTable) GetEventQueries(
	ctx context.Context,
	config_obj *config_proto.Config) ([]*actions_proto.VQLCollectorArgs, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	result := make([]*actions_proto.VQLCollectorArgs, 0, len(self.Events))
	result = append(result, self.Events...)

	// If there are no built in additional event artifacts we are done
	// - just run the queries from the event table.
	if config_obj.Client == nil ||
		len(config_obj.Client.AdditionalEventArtifacts) == 0 {
		return result, nil
	}

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return result, err
	}

	// Config enforced event queries are compiled using the built in
	// repository because we do no have access to the server
	// repository yet!
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return result, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return result, err
	}

	// Compile the built in artifacts
	queries, err := launcher.CompileCollectorArgs(ctx, config_obj,
		acl_managers.NullACLManager{}, repository,
		services.CompilerOptions{}, &flows_proto.ArtifactCollectorArgs{
			Artifacts: config_obj.Client.AdditionalEventArtifacts,
		})

	if err != nil {
		return result, err
	}

	result = append(result, queries...)
	return result, nil
}

func (self *EventTable) StartQueries(
	ctx context.Context,
	config_obj *config_proto.Config,
	output_chan chan *crypto_proto.VeloMessage) {

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)

	events, err := self.GetEventQueries(ctx, config_obj)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Error("While appending initial event artifacts: %v", err)
	}

	// Start a new query for each event.
	for _, event := range events {

		// Name of the query we are running. There must be at least
		// one query with a name.
		artifact_name := utils.GetQueryName(event.Query)
		if artifact_name == "" {
			continue
		}

		logger.Info("<green>Starting</> monitoring query %s", artifact_name)
		query_responder := responder.NewMonitoringResponder(
			ctx, config_obj, self.monitoring_manager,
			output_chan, artifact_name, event.QueryId)

		self.wg.Add(1)
		go func(event *actions_proto.VQLCollectorArgs) {
			defer self.wg.Done()
			defer query_responder.Close()

			// Event tables get refreshed by default every 12 hours.
			if event.Timeout == 0 {
				event.Timeout = 12 * 60 * 60
			}

			// Dont heartbeat too often for event queries
			// - the log generates un-neccesary traffic.
			if event.Heartbeat == 0 {
				event.Heartbeat = 300 // 5 minutes
			}

			// Start the query - if it is an event query this will
			// never complete until it is cancelled.
			self.RunQuery(self.Ctx, config_obj,
				artifact_name, query_responder, event)
			if artifact_name != "" {
				logger.Info("Finished monitoring query %s", artifact_name)
			}
		}(proto.Clone(event).(*actions_proto.VQLCollectorArgs))
	}
}

func (self *EventTable) RunQuery(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact_name string,
	query_responder responder.Responder,
	event *actions_proto.VQLCollectorArgs) {

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	refresh_timeout := event.Timeout
	event.Timeout = 999999

	for {
		sub_ctx, cancel := context.WithCancel(ctx)

		refresh := utils.Jitter(time.Second * time.Duration(refresh_timeout))

		// Start the query - if it is an event query this will not
		// complete until we cancell it due to refresh. If it is not
		// an event query, it will complete sooner but we wont start
		// it again until the refresh time.
		wg.Add(1)
		go func() {
			defer wg.Done()

			query_responder.Log(ctx, logging.DEBUG,
				fmt.Sprintf("Starting monitoring query %s with refresh in %v",
					artifact_name, refresh.Round(2).String()))

			action_obj := &VQLClientAction{}
			action_obj.StartQuery(
				config_obj, sub_ctx, query_responder, event)
		}()

		select {
		// Exit completely when the parent ctx is done.
		case <-ctx.Done():
			cancel()
			return

			// When the deadline fires, we refresh the query.
		case <-time.After(refresh):
			query_responder.Log(ctx, logging.DEBUG,
				fmt.Sprintf("Refreshing monitoring query %s", artifact_name))
			cancel()

			// Wait here for it to be done.
			wg.Wait()
		}
	}
}

func (self *EventTable) StartFromWriteback(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	output_chan chan *crypto_proto.VeloMessage) {

	// Get the event table from the writeback if possible.
	var event_table *actions_proto.VQLEventTable

	writeback_service := writeback.GetWritebackService()
	writeback, err := writeback_service.GetWriteback(config_obj)
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

	writeback_service := writeback.GetWritebackService()

	// No change required, skip it.
	if !changed {
		// We still need to write the new version
		err = writeback_service.MutateWriteback(config_obj,
			func(wb *config_proto.Writeback) error {
				wb.EventQueries = update_table
				return writeback.WritebackUpdateLevel2
			})
		if err != nil {
			responder.MakeErrorResponse(output_chan, "F.Monitoring",
				fmt.Sprintf("Unable to write events to writeback: %v", err))
		}
		return
	}

	// Kick off the queries
	self.StartQueries(ctx, config_obj, output_chan)

	// Update the writeback
	err = writeback_service.MutateWriteback(config_obj,
		func(wb *config_proto.Writeback) error {
			wb.EventQueries = update_table
			return writeback.WritebackUpdateLevel2
		})
	if err != nil {
		responder.MakeErrorResponse(output_chan, "F.Monitoring",
			fmt.Sprintf("Unable to write events to writeback: %v", err))
		return
	}
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
