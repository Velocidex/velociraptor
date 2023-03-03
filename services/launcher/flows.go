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
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Filter will be applied on flows to remove those we dont care about.
func (self *Launcher) GetFlows(
	config_obj *config_proto.Config,
	client_id string, include_archived bool,
	flow_filter func(flow *flows_proto.ArtifactCollectorContext) bool,
	offset uint64, length uint64) (*api_proto.ApiFlowResponse, error) {

	result := &api_proto.ApiFlowResponse{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, "")
	var flow_urns []api.DSPathSpec
	all_flow_urns, err := db.ListChildren(
		config_obj, flow_path_manager.ContainerPath())
	if err != nil {
		return nil, err
	}

	seen := make(map[string]api.DSPathSpec)

	// We only care about the flow contexts
	for _, urn := range all_flow_urns {
		// We really prefer the more modern JSON datastore objects but
		// we do support older protobuf based objects if they are
		// there.
		if urn.Type() == api.PATH_TYPE_DATASTORE_JSON {
			seen[urn.Base()] = urn
			continue
		}

		if urn.Type() == api.PATH_TYPE_DATASTORE_PROTO {
			_, pres := seen[urn.Base()]
			if !pres {
				seen[urn.Base()] = urn
			}
		}
	}

	for _, v := range seen {
		flow_urns = append(flow_urns, v)
	}

	// No flows were returned.
	if len(flow_urns) == 0 {
		return result, nil
	}

	// Flow IDs represent timestamp so they are sortable. The UI
	// relies on more recent flows being at the top.
	sort.Slice(flow_urns, func(i, j int) bool {
		return flow_urns[i].Base() >= flow_urns[j].Base()
	})

	// Page the flow urns
	end := offset + length
	if end > uint64(len(flow_urns)) {
		end = uint64(len(flow_urns))
	}
	flow_urns = flow_urns[offset:end]

	// Collect the items that match from this backend read
	// into an array
	items := []*flows_proto.ArtifactCollectorContext{}

	for _, urn := range flow_urns {
		// Hide the monitoring flow since it is not a real flow.
		if urn.Base() == constants.MONITORING_WELL_KNOWN_FLOW {
			continue
		}

		collection_context, err := LoadCollectionContext(
			config_obj, client_id, urn.Base())
		if err != nil {
			continue
		}

		if flow_filter != nil && !flow_filter(collection_context) {
			continue
		}

		items = append(items, collection_context)
	}

	result.Items = items
	return result, nil
}

// Gets more detailed information about the flow context - fills in
// availableDownloads etc.
func (self *Launcher) GetFlowDetails(
	config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.FlowDetails, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.FlowDetails{}, nil
	}

	collection_context, err := LoadCollectionContext(config_obj, client_id, flow_id)
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

// Load the collector context from storage.
func LoadCollectionContext(
	config_obj *config_proto.Config,
	client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error) {

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	collection_context := &flows_proto.ArtifactCollectorContext{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	err = db.GetSubject(
		config_obj, flow_path_manager.Path(), collection_context)
	if err != nil {
		return nil, err
	}

	if collection_context.SessionId == "" {
		return nil, errors.New("Unknown flow " + client_id + " " + flow_id)
	}

	// Try to open the stats context
	stats_context := &flows_proto.ArtifactCollectorContext{}
	err = db.GetSubject(
		config_obj, flow_path_manager.Stats(), stats_context)
	if err != nil {
		UpdateFlowStats(collection_context)
		return collection_context, nil
	}

	// Copy relevant fields into the main context
	if stats_context.TotalUploadedFiles > 0 {
		collection_context.TotalUploadedFiles = stats_context.TotalUploadedFiles
	}

	if stats_context.TotalExpectedUploadedBytes > 0 {
		collection_context.TotalExpectedUploadedBytes = stats_context.TotalExpectedUploadedBytes
	}

	if stats_context.TotalUploadedBytes > 0 {
		collection_context.TotalUploadedBytes = stats_context.TotalUploadedBytes
	}

	if stats_context.TotalCollectedRows > 0 {
		collection_context.TotalCollectedRows = stats_context.TotalCollectedRows
	}

	if stats_context.TotalLogs > 0 {
		collection_context.TotalLogs = stats_context.TotalLogs
	}

	if stats_context.ActiveTime > 0 {
		collection_context.ActiveTime = stats_context.ActiveTime
	}

	if len(stats_context.ArtifactsWithResults) > 0 {
		collection_context.ArtifactsWithResults = stats_context.ArtifactsWithResults
	}

	if len(stats_context.QueryStats) > 0 {
		collection_context.QueryStats = stats_context.QueryStats
	}

	UpdateFlowStats(collection_context)

	return collection_context, nil
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

	collection_context, err := LoadCollectionContext(
		config_obj, client_id, flow_id)
	if err == nil {
		if collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
			return nil, errors.New("Flow is not in the running state. " +
				"Can only cancel running flows.")
		}

		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		collection_context.Status = "Cancelled by " + username
		collection_context.Backtrace = ""

		flow_path_manager := paths.NewFlowPathManager(
			collection_context.ClientId, collection_context.SessionId)

		db, err := datastore.GetDB(config_obj)
		if err == nil {
			db.SetSubjectWithCompletion(
				config_obj, flow_path_manager.Path(),
				collection_context, nil)
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

func (self *Launcher) GetFlowRequests(
	config_obj *config_proto.Config,
	client_id string, flow_id string,
	offset uint64, count uint64) (*api_proto.ApiFlowRequestDetails, error) {
	if count == 0 {
		count = 50
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	result := &api_proto.ApiFlowRequestDetails{}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	flow_details := &api_proto.ApiFlowRequestDetails{}
	err = db.GetSubject(
		config_obj, flow_path_manager.Task(), flow_details)
	if err != nil {
		return nil, err
	}

	if offset > uint64(len(flow_details.Items)) {
		return result, nil
	}

	end := offset + count
	if end > uint64(len(flow_details.Items)) {
		end = uint64(len(flow_details.Items))
	}

	result.Items = flow_details.Items[offset:end]
	return result, nil
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

func (self *Launcher) WriteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	flow *flows_proto.ArtifactCollectorContext) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	flow_path_manager := paths.NewFlowPathManager(flow.ClientId, flow.SessionId)
	return db.SetSubjectWithCompletion(
		config_obj, flow_path_manager.Path(),
		flow, utils.BackgroundWriter)
}
