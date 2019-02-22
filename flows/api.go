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
	"path"
	"strings"

	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/responder"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

func GetFlows(
	config_obj *api_proto.Config,
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
	config_obj *api_proto.Config,
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

	return &api_proto.ApiFlow{
		Urn:        *flow_urn,
		ClientId:   client_id,
		FlowId:     flow_id,
		Name:       flow_obj.RunnerArgs.FlowName,
		RunnerArgs: flow_obj.RunnerArgs,
		Context:    flow_obj.FlowContext,
	}, nil
}

func GetFlowRequests(
	config_obj *api_proto.Config,
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
