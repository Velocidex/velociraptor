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
package launcher

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Filter will be applied on flows to remove those we dont care about.
func (self *Launcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string,
	options result_sets.ResultSetOptions,
	offset, length int64) (*api_proto.ApiFlowResponse, error) {

	result := &api_proto.ApiFlowResponse{}
	flow_summaries, total_rows, err := self.Storage().ListFlows(
		ctx, config_obj, client_id, options, offset, length)
	if err != nil {
		return nil, err
	}

	// No flows were returned.
	if len(flow_summaries) == 0 {
		return result, nil
	}

	result.Total = uint64(total_rows)

	// Collect the items that match from this backend read into an
	// array
	items := []*flows_proto.ArtifactCollectorContext{}

	for _, flow_summary := range flow_summaries {
		collection_context, err := self.Storage().
			LoadCollectionContext(ctx, config_obj,
				client_id, flow_summary.FlowId)
		if err == nil {
			// Remove certain fields that are not necessary.
			if collection_context.Request != nil {
				collection_context.Request.CompiledCollectorArgs = nil
			}
			items = append(items, collection_context)
		}
	}

	result.Items = items

	return result, nil
}

// Gets more detailed information about the flow context - fills in
// availableDownloads etc.
func (self *Launcher) GetFlowDetails(
	ctx context.Context,
	config_obj *config_proto.Config,
	opts services.GetFlowOptions,
	client_id string, flow_id string) (*api_proto.FlowDetails, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.FlowDetails{}, nil
	}

	collection_context, err := self.Storage().LoadCollectionContext(
		ctx, config_obj, client_id, flow_id)
	if err != nil {
		return nil, err
	}

	res := &api_proto.FlowDetails{
		Context: collection_context,
	}

	// Include the AvailableDownloads
	if opts.Downloads {
		res.AvailableDownloads, _ = availableDownloadFiles(ctx, config_obj, client_id, flow_id)
	}
	return res, nil
}

// availableDownloads returns the prepared zip downloads available to
// be fetched by the user at this moment.
func availableDownloadFiles(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.AvailableDownloads, error) {

	export_manager, err := services.GetExportManager(config_obj)
	if err != nil {
		return nil, err
	}

	return export_manager.GetAvailableDownloadFiles(ctx,
		config_obj, services.ContainerOptions{
			Type:     services.FlowExport,
			ClientId: client_id,
			FlowId:   flow_id,
		})
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
		switch collection_context.State {
		case flows_proto.ArtifactCollectorContext_RUNNING,
			flows_proto.ArtifactCollectorContext_WAITING,
			flows_proto.ArtifactCollectorContext_IN_PROGRESS,
			flows_proto.ArtifactCollectorContext_UNRESPONSIVE:

		default:
			//			return nil, errors.New("Flow is not in the running state. " +
			//	"Can only cancel running flows.")
		}

		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		old_status := collection_context.Status
		new_status := "Cancelled by " + username
		if old_status != "" {
			collection_context.Status = new_status + ": " + old_status
		} else {
			collection_context.Status = new_status
			collection_context.Backtrace = ""
		}

		err := self.Storage().WriteFlow(
			ctx, config_obj, collection_context, utils.BackgroundWriter)
		if err != nil {
			return nil, err
		}
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
			Urgent:    true,
			Cancel:    cancel_msg,
			SessionId: flow_id,
		}, services.NOTIFY_CLIENT, nil)
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
	if len(collection_context.QueryStats) == 0 &&
		collection_context.InflightTime == 0 {
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

		if s.ErrorMessage != "" && collection_context.Status == "" {
			collection_context.Status = s.ErrorMessage
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
	if collection_context.TotalRequests > 0 &&
		collection_context.OutstandingRequests <= 0 &&
		collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING {
		collection_context.State = flows_proto.ArtifactCollectorContext_FINISHED
	}

	// The flow has been scheduled - either indicate it as waiting, in
	// progress or unresponsive.
	if (collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING ||
		collection_context.State == flows_proto.ArtifactCollectorContext_IN_PROGRESS) &&
		collection_context.InflightTime > 0 {

		// If the client did not send any status updates yet the query
		// is not running yet - lets put it in the WAITING state.
		if len(collection_context.QueryStats) == 0 {
			collection_context.State = flows_proto.ArtifactCollectorContext_WAITING

		} else {

			// Determine how long ago did we get an update?
			now := uint64(utils.GetTime().Now().Unix())
			if now-collection_context.InflightTime > 300 {
				collection_context.State = flows_proto.ArtifactCollectorContext_UNRESPONSIVE
			} else {
				collection_context.State = flows_proto.ArtifactCollectorContext_IN_PROGRESS
			}
		}
	}

}

func (self *Launcher) Storage() services.FlowStorer {
	return self.Storage_
}
