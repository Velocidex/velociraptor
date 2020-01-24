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
package flows

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/vfilter"
)

func GetFlows(
	config_obj *config_proto.Config,
	client_id string, include_archived bool,
	offset uint64, length uint64) (*api_proto.ApiFlowResponse, error) {

	result := &api_proto.ApiFlowResponse{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_urns, err := db.ListChildren(
		config_obj, path.Dir(GetCollectionPath(client_id, "X")),
		offset, length)
	if err != nil {
		return nil, err
	}

	for _, urn := range flow_urns {
		// Hide the monitoring flow since it is not a real flow.
		if strings.HasSuffix(urn, constants.MONITORING_WELL_KNOWN_FLOW) {
			continue
		}

		collection_context := &flows_proto.ArtifactCollectorContext{}
		err := db.GetSubject(config_obj, urn, collection_context)
		if err != nil {
			logging.GetLogger(
				config_obj, &logging.FrontendComponent).
				Error("Unable to open collection", err)
			continue
		}

		if !include_archived &&
			collection_context.State ==
				flows_proto.ArtifactCollectorContext_ARCHIVED {
			continue
		}

		result.Items = append(result.Items, collection_context)
	}
	return result, nil
}

func GetFlowDetails(
	config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.FlowDetails, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.FlowDetails{}, nil
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	urn := GetCollectionPath(client_id, flow_id)
	collection_context := &flows_proto.ArtifactCollectorContext{}
	err = db.GetSubject(config_obj, urn, collection_context)
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

	download_file := artifacts.GetDownloadsFile(client_id, flow_id)
	download_path := path.Dir(download_file)

	return getAvailableDownloadFiles(config_obj, download_path)
}

func getAvailableDownloadFiles(config_obj *config_proto.Config,
	download_path string) (*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	file_store_factory := file_store.GetFileStore(config_obj)
	files, err := file_store_factory.ListDirectory(download_path)
	if err != nil {
		return nil, err
	}

	is_complete := func(name string) bool {
		for _, item := range files {
			if item.Name() == name+".lock" {
				return false
			}
		}
		return true
	}

	for _, item := range files {
		if strings.HasSuffix(item.Name(), ".lock") {
			continue
		}

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     item.Name(),
			Path:     path.Join(download_path, item.Name()),
			Size:     uint64(item.Size()),
			Date:     fmt.Sprintf("%v", item.ModTime()),
			Complete: is_complete(item.Name()),
		})
	}

	return result, nil
}

func CancelFlow(
	config_obj *config_proto.Config,
	client_id, flow_id, username string,
	api_client_factory grpc_client.APIClientFactory) (
	*api_proto.StartFlowResponse, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.StartFlowResponse{}, nil
	}

	collection_context, err := LoadCollectionContext(
		config_obj, client_id, flow_id)
	if err == nil {
		defer closeContext(config_obj, collection_context)

		if collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
			return nil, errors.New("Flow is not in the running state. " +
				"Can only cancel running flows.")
		}

		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		collection_context.Status = "Cancelled by " + username
		collection_context.Backtrace = ""
		collection_context.Dirty = true
	}

	// Get all queued tasks for the client and delete only those in this flow.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	tasks, err := db.GetClientTasks(config_obj, client_id,
		true /* do_not_lease */)
	if err != nil {
		return nil, err
	}

	// Cancel all the tasks
	for _, task := range tasks {
		if task.SessionId == flow_id {
			err = db.UnQueueMessageForClient(config_obj, client_id, task)
			if err != nil {
				return nil, err
			}
		}
	}

	// Queue a cancellation message to the client for this flow
	// id.
	err = db.QueueMessageForClient(config_obj, client_id,
		&crypto_proto.GrrMessage{
			Cancel:    &crypto_proto.Cancel{},
			SessionId: flow_id,
		})
	if err != nil {
		return nil, err
	}

	client, cancel := api_client_factory.GetAPIClient(config_obj)
	defer cancel()

	_, err = client.NotifyClients(context.Background(),
		&api_proto.NotificationRequest{
			ClientId: client_id,
		})
	if err != nil {
		return nil, err
	}

	return &api_proto.StartFlowResponse{
		FlowId: flow_id,
	}, nil
}

func ArchiveFlow(
	config_obj *config_proto.Config,
	client_id string, flow_id string, username string) (
	*api_proto.StartFlowResponse, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.StartFlowResponse{}, nil
	}

	collection_context, err := LoadCollectionContext(
		config_obj, client_id, flow_id)
	if err != nil {
		return nil, err
	}
	defer closeContext(config_obj, collection_context)

	if collection_context.State != flows_proto.ArtifactCollectorContext_TERMINATED &&
		collection_context.State != flows_proto.ArtifactCollectorContext_ERROR {
		return nil, errors.New("Flow must be stopped before it can be archived.")
	}

	collection_context.State = flows_proto.ArtifactCollectorContext_ARCHIVED
	collection_context.Status = "Archived by " + username
	collection_context.Backtrace = ""
	collection_context.Dirty = true

	// Keep track of archived flows so they can be purged later.
	row := ordereddict.NewDict().
		Set("Timestamp", time.Now().UTC().Unix()).
		Set("Flow", collection_context)
	serialized, err := json.Marshal([]vfilter.Row{row})
	if err == nil {
		GJournalWriter.Channel <- &Event{
			Config:    config_obj,
			ClientId:  client_id,
			QueryName: "System.Flow.Archive",
			Response:  string(serialized),
			Columns:   []string{"Timestamp", "Flow"},
		}
	}

	return &api_proto.StartFlowResponse{
		FlowId: flow_id,
	}, nil
}

func GetFlowRequests(
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

	flow_details := &api_proto.ApiFlowRequestDetails{}
	err = db.GetSubject(config_obj,
		path.Join(GetCollectionPath(client_id, flow_id), "task"),
		flow_details)
	if err != nil {
		return nil, err
	}

	// Set the task_id in the details protobuf.
	set_task_id_in_details := func(request_id, task_id uint64) bool {
		for _, item := range flow_details.Items {
			if item.RequestId == request_id {
				item.TaskId = task_id
				return true
			}
		}

		return false
	}

	requests, err := db.GetClientTasks(config_obj, client_id, true)
	if err != nil {
		return nil, err
	}
	for _, request := range requests {
		set_task_id_in_details(request.RequestId, request.TaskId)
	}

	if offset > uint64(len(flow_details.Items)) {
		return result, nil
	}

	end := offset + count
	if end > uint64(len(flow_details.Items)) {
		end = uint64(len(flow_details.Items))
	}

	result.Items = flow_details.Items[offset:end]

	// Remove unimportant fields
	for _, item := range result.Items {
		item.SessionId = ""
		item.RequestId = 0
	}

	return result, nil
}
