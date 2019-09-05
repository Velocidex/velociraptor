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
	"fmt"
	"path"
	"strings"

	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

func GetFlows(
	config_obj *config_proto.Config,
	client_id string,
	offset uint64, length uint64) (*api_proto.ApiFlowResponse, error) {

	result := &api_proto.ApiFlowResponse{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_urns, err := db.ListChildren(
		config_obj, urns.BuildURN(
			"clients", client_id, "flows"),
		offset, length)
	if err != nil {
		return nil, err
	}

	for _, urn := range flow_urns {
		flow_obj, err := GetAFF4FlowObject(config_obj, urn)
		if err != nil {
			// Skip flows we can not load any more.
			logging.GetLogger(
				config_obj, &logging.FrontendComponent).
				Error("", err)
			continue
		}

		// Skip system flows - they are hidden from users
		// because they are internal and users cant interact
		// with them anyway.
		flow_id := path.Base(urn)
		if flow_id == "F.Monitoring" {
			continue
		}

		if flow_obj.RunnerArgs != nil {
			item := &api_proto.ApiFlow{
				Urn:        urn,
				ClientId:   client_id,
				FlowId:     flow_id,
				Name:       flow_obj.RunnerArgs.FlowName,
				RunnerArgs: flow_obj.RunnerArgs,
				Context:    flow_obj.FlowContext,
			}
			result.Items = append(result.Items, item)
		}
	}
	return result, nil
}

func GetFlowDetails(
	config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.ApiFlow, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.ApiFlow{}, nil
	}

	flow_urn, err := ValidateFlowId(client_id, flow_id)
	if err != nil {
		return nil, err
	}

	flow_obj, err := GetAFF4FlowObject(config_obj, *flow_urn)
	if err != nil {
		return nil, err
	}

	availableDownloads, _ := availableDownloadFiles(config_obj, client_id, flow_id)
	return &api_proto.ApiFlow{
		Urn:                *flow_urn,
		ClientId:           client_id,
		FlowId:             flow_id,
		Name:               flow_obj.RunnerArgs.FlowName,
		RunnerArgs:         flow_obj.RunnerArgs,
		Context:            flow_obj.FlowContext,
		AvailableDownloads: availableDownloads,
	}, nil
}

func availableDownloadFiles(config_obj *config_proto.Config,
	client_id string, flow_id string) (*api_proto.AvailableDownloads, error) {

	result := &api_proto.AvailableDownloads{}
	download_file := artifacts.GetDownloadsFile(client_id, flow_id)
	download_path := path.Dir(download_file)

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
	client_id string, flow_id string, username string) (
	*api_proto.StartFlowResponse, error) {
	if flow_id == "" || client_id == "" {
		return &api_proto.StartFlowResponse{}, nil
	}

	flow_urn, err := ValidateFlowId(client_id, flow_id)
	if err != nil {
		return nil, err
	}

	flow_obj, err := GetAFF4FlowObject(config_obj, *flow_urn)
	if err != nil {
		return nil, err
	}

	if flow_obj.FlowContext != nil {
		if flow_obj.FlowContext.State != flows_proto.FlowContext_RUNNING {
			return nil, errors.New("Flow is not in the running state. " +
				"Can only cancel running flows.")
		}

		flow_obj.FlowContext.State = flows_proto.FlowContext_ERROR
		flow_obj.FlowContext.Status = "Cancelled by " + username
		flow_obj.FlowContext.Backtrace = ""
		flow_obj.dirty = true
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// Get all queued tasks for the client and delete only those in this flow.
	tasks, err := db.GetClientTasks(config_obj, client_id, true /* do_not_lease */)
	if err != nil {
		return nil, err
	}

	session_id := urns.BuildURN("clients", client_id, "flows", flow_id)
	for _, task := range tasks {
		if task.SessionId == session_id {
			err = db.UnQueueMessageForClient(config_obj, client_id, task)
			if err != nil {
				return nil, err
			}
		}
	}

	err = SetAFF4FlowObject(config_obj, flow_obj)
	if err != nil {
		return nil, err
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
	result := &api_proto.ApiFlowRequestDetails{}

	session_id := urns.BuildURN("clients", client_id, "flows", flow_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	requests, err := db.GetClientTasks(config_obj, client_id, true)
	if err != nil {
		return nil, err
	}
	for idx, request := range requests {
		if idx < int(offset) {
			continue
		}

		if idx > int(offset+count) {
			break
		}

		if request.SessionId == session_id {
			args := responder.ExtractGrrMessagePayload(request)
			payload, err := ptypes.MarshalAny(args)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			request.Payload = payload
			request.Args = nil
			request.ArgsRdfName = ""
			result.Items = append(result.Items, request)
		}
	}

	return result, nil
}

func GetFlowDescriptors() (*api_proto.FlowDescriptors, error) {
	result := &api_proto.FlowDescriptors{}
	for _, item := range GetDescriptors() {
		if !item.Internal {
			result.Items = append(result.Items, item)
		}
	}

	return result, nil
}

func ValidateFlowId(client_id string, flow_id string) (*string, error) {
	base_flow := path.Base(flow_id)
	if !strings.HasPrefix(base_flow, constants.FLOW_PREFIX) {
		return nil, errors.New(
			"Flows must start with " + constants.FLOW_PREFIX)
	}

	rebuild_urn := urns.BuildURN("clients", client_id, "flows", base_flow)
	return &rebuild_urn, nil
}
