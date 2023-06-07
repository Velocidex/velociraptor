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
package launcher

import (
	"context"
	"sort"

	"github.com/go-errors/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Filter will be applied on flows to remove those we dont care about.
func (self *Launcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, include_archived bool,
	flow_filter func(flow *flows_proto.ArtifactCollectorContext) bool,
	offset uint64, length uint64) (*api_proto.ApiFlowResponse, error) {

	result := &api_proto.ApiFlowResponse{}
	flow_ids, err := self.Storage().ListFlows(ctx, config_obj, client_id)
	if err != nil {
		return nil, err
	}

	// No flows were returned.
	if len(flow_ids) == 0 {
		return result, nil
	}

	// The flow ids encode the creation time so we need to sort by
	// flow id **before** we page the results.  NOTE: Currently the
	// GUI will only show the top 1000 flows. If you have more than
	// that it will not show more flows.
	sort.Slice(flow_ids, func(i, j int) bool {
		return flow_ids[i] > flow_ids[j]
	})

	// Page the flow urns
	end := offset + length
	if end > uint64(len(flow_ids)) {
		end = uint64(len(flow_ids))
	}
	flow_ids = flow_ids[offset:end]

	flow_reader := NewFlowReader(
		ctx, config_obj, self.Storage(), client_id)
	go func() {
		defer flow_reader.Close()

		for _, flow_id := range flow_ids {
			flow_reader.In <- flow_id
		}
	}()

	// Collect the items that match from this backend read into an
	// array
	items := []*flows_proto.ArtifactCollectorContext{}

	for collection_context := range flow_reader.Out {
		if flow_filter != nil && !flow_filter(collection_context) {
			continue
		}
		items = append(items, collection_context)
	}

	// Flow IDs represent timestamp so they are sortable. The UI
	// relies on more recent flows being at the top.
	sort.Slice(items, func(i, j int) bool {
		return items[i].SessionId >= items[j].SessionId
	})

	result.Items = items
	return result, nil
}

// Gets more detailed information about the flow context - fills in
// availableDownloads etc.
func (self *Launcher) GetFlowDetails(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.FlowDetails, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.FlowDetails{}, nil
	}

	collection_context, err := self.Storage().LoadCollectionContext(
		ctx, config_obj, client_id, flow_id)
	if err != nil {
		return nil, err
	}

	availableDownloads, _ := availableDownloadFiles(config_obj, client_id, flow_id)
	return &api_proto.FlowDetails{
		Context:            collection_context,
		AvailableDownloads: availableDownloads,
	}, nil
}

// availableDownloads returns the prepared zip downloads available to
// be fetched by the user at this moment.
func availableDownloadFiles(config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.AvailableDownloads, error) {

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	download_dir := flow_path_manager.GetDownloadsDirectory()

	return reporting.GetAvailableDownloadFiles(config_obj, download_dir)
}

func (self *Launcher) CancelFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id, username string) (
	res *api_proto.StartFlowResponse, err error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.StartFlowResponse{}, nil
	}

	// Handle server collections especially via the server artifact
	// runner.
	if client_id == "server" {
		server_artifacts_service, err := services.GetServerArtifactRunner(
			config_obj)
		if err != nil {
			return nil, err
		}

		server_artifacts_service.Cancel(ctx, flow_id, username)
		return &api_proto.StartFlowResponse{
			FlowId: flow_id,
		}, nil
	}

	collection_context, err := self.Storage().LoadCollectionContext(
		ctx, config_obj, client_id, flow_id)
	if err == nil {
		if collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
			return nil, errors.New("Flow is not in the running state. " +
				"Can only cancel running flows.")
		}

		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		collection_context.Status = "Cancelled by " + username
		collection_context.Backtrace = ""

		self.Storage().WriteFlow(
			ctx, config_obj, collection_context, utils.BackgroundWriter)
	}

	// Get all queued tasks for the client and delete only those in this flow.
	client_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil, err
	}

	// Get all the tasks but only dequeue the ones intended for the
	// cancelled flow.
	tasks, err := client_manager.PeekClientTasks(ctx, client_id)
	if err != nil {
		return nil, err
	}

	// Cancel all the relevant tasks
	for _, task := range tasks {
		if task.SessionId == flow_id {
			err = client_manager.UnQueueMessageForClient(ctx, client_id, task)
			if err != nil {
				return nil, err
			}
		}
	}

	// Queue a cancellation message to the client for this flow
	// id.
	cancel_msg := &crypto_proto.Cancel{}
	if client_id == "server" {
		// Only include the principal on server messages so the
		// server_artifacts service can log the principal. No need to
		// forward to the client.
		cancel_msg.Principal = username
	}

	err = client_manager.QueueMessageForClient(ctx, client_id,
		&crypto_proto.VeloMessage{
			Cancel:    cancel_msg,
			SessionId: flow_id,
		}, services.NOTIFY_CLIENT, utils.BackgroundWriter)
	if err != nil {
		return nil, err
	}

	return &api_proto.StartFlowResponse{
		FlowId: flow_id,
	}, nil
}

// The collection_context contains high level stats that summarise the
// colletion. We derive this information from the specific results of
// each query.
func UpdateFlowStats(collection_context *flows_proto.ArtifactCollectorContext) {
	// Support older colletions which do not have this info
	if len(collection_context.QueryStats) == 0 {
		return
	}

	// Now update the overall collection statuses based on all the
	// individual query status. The collection status is a high level
	// overview of the entire collection.
	if collection_context.State == flows_proto.ArtifactCollectorContext_UNSET {
		collection_context.State = flows_proto.ArtifactCollectorContext_RUNNING
	}

	// Total execution duration is the sum of all the query durations
	// (this can be faster than wall time if queries run in parallel)
	collection_context.ExecutionDuration = 0
	collection_context.TotalUploadedBytes = 0
	collection_context.TotalExpectedUploadedBytes = 0
	collection_context.TotalUploadedFiles = 0
	collection_context.TotalCollectedRows = 0
	collection_context.TotalLogs = 0
	collection_context.ActiveTime = 0
	collection_context.StartTime = 0

	// Number of queries completed.
	completed_count := 0

	for _, s := range collection_context.QueryStats {
		// The ExecutionDuration represents the longest query that
		// ran. It should be the same as the ActiveTime - StartTime
		if s.Duration > collection_context.ExecutionDuration {
			collection_context.ExecutionDuration = s.Duration
		}
		collection_context.TotalUploadedBytes += uint64(s.UploadedBytes)
		collection_context.TotalExpectedUploadedBytes += uint64(s.ExpectedUploadedBytes)
		collection_context.TotalUploadedFiles += uint64(s.UploadedFiles)
		collection_context.TotalCollectedRows += uint64(s.ResultRows)
		collection_context.TotalLogs += uint64(s.LogRows)

		if s.LastActive > collection_context.ActiveTime {
			collection_context.ActiveTime = s.LastActive
		}

		if collection_context.StartTime == 0 ||
			collection_context.StartTime > s.FirstActive {
			collection_context.StartTime = s.FirstActive
		}

		// Get the first errored query and mark the entire collection_context with it.
		if collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING &&
			s.Status == crypto_proto.VeloStatus_GENERIC_ERROR {
			collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
			collection_context.Status = s.ErrorMessage
			collection_context.Backtrace = s.Backtrace
		}

		// Query is considered complete if it is in the ERROR or OK state
		if s.Status == crypto_proto.VeloStatus_OK ||
			s.Status == crypto_proto.VeloStatus_GENERIC_ERROR {
			completed_count++
		}

		// Merge the NamesWithResponse for all the queries.
		for _, a := range s.NamesWithResponse {
			if a != "" &&
				!utils.InString(collection_context.ArtifactsWithResults, a) {
				collection_context.ArtifactsWithResults = append(
					collection_context.ArtifactsWithResults, a)
			}
		}
	}

	// How many queries are outstanding still?
	collection_context.TotalRequests = int64(len(collection_context.QueryStats))
	collection_context.OutstandingRequests = collection_context.TotalRequests -
		int64(completed_count)

	// All queries are accounted for.
	if collection_context.OutstandingRequests <= 0 &&
		collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING {
		collection_context.State = flows_proto.ArtifactCollectorContext_FINISHED
	}
}

func (self *Launcher) Storage() services.FlowStorer {
	return self.Storage_
}
