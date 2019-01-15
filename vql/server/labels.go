package server

import (
	"context"

	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type AddLabelsArgs struct {
	ClientId string   `vfilter:"required,field=client_id"`
	Labels   []string `vfilter:"required,field=labels"`
	Op       string   `vfilter:"optional,field=op"`
}

type AddLabels struct{}

func (self *AddLabels) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &AddLabelsArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("label: %s", err.Error())
		return vfilter.Null{}
	}

	any_config_obj, _ := scope.Resolve("server_config")
	config_obj, ok := any_config_obj.(*api_proto.Config)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	request := &api_proto.LabelClientsRequest{
		Labels:    arg.Labels,
		ClientIds: []string{arg.ClientId},
		Operation: arg.Op,
	}

	_, err = api.LabelClients(config_obj, request)
	if err != nil {
		return vfilter.Null{}
	}

	return arg
}

func (self AddLabels) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "label",
		Doc: "Add the labels to the client. " +
			"If op is 'remove' then remove these labels.",
		ArgType: type_map.AddType(scope, &AddLabelsArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AddLabels{})
}
