package api

import (
	"fmt"
	"path"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows "www.velocidex.com/golang/velociraptor/flows"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

func getFlows(
	config_obj *config.Config,
	client_id string,
	offset uint64, length uint64) (*api_proto.ApiFlowResponse, error) {

	result := &api_proto.ApiFlowResponse{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	flow_urns, err := db.ListChildren(
		config_obj, fmt.Sprintf("aff4:/%s/flows", client_id),
		offset, length)
	if err != nil {
		return nil, err
	}

	for _, urn := range flow_urns {
		flow_obj, err := flows.GetAFF4FlowObject(config_obj, urn)
		if err != nil {
			return nil, err
		}

		item := &api_proto.ApiFlow{
			Urn:        urn,
			ClientId:   client_id,
			FlowId:     path.Base(urn),
			Name:       flow_obj.RunnerArgs.FlowName,
			RunnerArgs: flow_obj.RunnerArgs,
			Context:    flow_obj.FlowContext,
		}

		result.Items = append(result.Items, item)
	}

	utils.Debug(result)

	return result, nil
}

func getFlowDetails(
	config_obj *config.Config,
	client_id string, flow_id string) (*api_proto.ApiFlow, error) {

	flow_urn := fmt.Sprintf("aff4:/%s/flows/%s", client_id, flow_id)
	utils.Debug(flow_urn)

	flow_obj, err := flows.GetAFF4FlowObject(config_obj, flow_urn)
	if err != nil {
		return nil, err
	}

	return &api_proto.ApiFlow{
		Urn:        flow_urn,
		ClientId:   client_id,
		FlowId:     flow_id,
		Name:       flow_obj.RunnerArgs.FlowName,
		RunnerArgs: flow_obj.RunnerArgs,
		Context:    flow_obj.FlowContext,
	}, nil
}
