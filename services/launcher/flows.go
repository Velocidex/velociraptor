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
	"time"

	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
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

	// We only care about the flow contexts
	for _, urn := range all_flow_urns {
		if !urn.IsDir() {
			flow_urns = append(flow_urns, urn)
		}
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

		collection_context := &flows_proto.ArtifactCollectorContext{}
		err := db.GetSubject(config_obj, urn, collection_context)
		if err != nil || collection_context.SessionId == "" {
			logging.GetLogger(
				config_obj, &logging.FrontendComponent).
				Error("Unable to open collection: %v", err)
			continue
		}

		if !include_archived &&
			collection_context.State ==
				flows_proto.ArtifactCollectorContext_ARCHIVED {
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

	ping := &flows_proto.PingContext{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	err = db.GetSubject(config_obj, flow_path_manager.Ping(), ping)
	if err == nil && ping.ActiveTime > collection_context.ActiveTime {
		collection_context.ActiveTime = ping.ActiveTime
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

	return getAvailableDownloadFiles(config_obj, download_dir)
}

func getAvailableDownloadFiles(config_obj *config_proto.Config,
	download_dir api.FSPathSpec) (*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	file_store_factory := file_store.GetFileStore(config_obj)
	files, err := file_store_factory.ListDirectory(download_dir)
	if err != nil {
		return nil, err
	}

	is_complete := func(name string) bool {
		for _, item := range files {
			ps := item.PathSpec()
			// If there is a lock file we are not done.
			if ps.Base() == name &&
				ps.Type() == api.PATH_TYPE_FILESTORE_LOCK {
				return false
			}
		}
		return true
	}

	for _, item := range files {
		ps := item.PathSpec()

		// Skip lock files
		if ps.Type() == api.PATH_TYPE_FILESTORE_LOCK {
			continue
		}

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     item.Name(),
			Type:     api.GetExtensionForFilestore(ps),
			Path:     ps.AsClientPath(),
			Size:     uint64(item.Size()),
			Date:     item.ModTime().UTC().Format(time.RFC3339),
			Complete: is_complete(ps.Base()),
		})
	}

	return result, nil
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
	err = client_manager.QueueMessageForClient(ctx, client_id,
		&crypto_proto.VeloMessage{
			Cancel:    &crypto_proto.Cancel{},
			SessionId: flow_id,
		}, true /* notify */, nil)
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
