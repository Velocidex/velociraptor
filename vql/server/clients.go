// VQL plugins for running on the server.

package server

import (
	"context"
	"path"

	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/urns"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ClientsPluginArgs struct {
	Search string `vfilter:"optional,field=search"`
}

type ClientsPlugin struct{}

func (self ClientsPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		arg := &ClientsPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("clients: %v", err)
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*config.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		search := arg.Search
		if search == "" {
			search = "all"
		}

		for _, client_id := range db.SearchClients(
			config_obj, constants.CLIENT_INDEX_URN,
			search, 0, 1000000) {
			api_client, err := api.GetApiClient(config_obj, client_id, false)
			if err == nil {
				output_chan <- api_client
			}
		}
	}()

	return output_chan
}

func (self ClientsPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "clients",
		Doc:     "Retrieve the list of clients.",
		RowType: type_map.AddType(scope, &api_proto.ApiClient{}),
		ArgType: type_map.AddType(scope, &ClientsPluginArgs{}),
	}
}

type FlowsPluginArgs struct {
	ClientId []string `vfilter:"required,field=client_id"`
}

type FlowsPlugin struct{}

func (self FlowsPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		arg := &FlowsPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("flows: %v", err)
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*config.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		for _, client_id := range arg.ClientId {
			flow_urns, err := db.ListChildren(
				config_obj, urns.BuildURN(
					"clients", client_id, "flows"),
				0, 10000)
			if err != nil {
				return
			}

			for _, urn := range flow_urns {
				flow_obj, err := flows.GetAFF4FlowObject(config_obj, urn)
				if err != nil {
					continue
				}

				item := &api_proto.ApiFlow{
					Urn:        urn,
					ClientId:   client_id,
					FlowId:     path.Base(urn),
					Name:       flow_obj.RunnerArgs.FlowName,
					RunnerArgs: flow_obj.RunnerArgs,
					Context:    flow_obj.FlowContext,
				}

				output_chan <- item

			}
		}
	}()

	return output_chan
}

func (self FlowsPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "flows",
		Doc:     "Retrieve the flows launched on each client.",
		RowType: type_map.AddType(scope, &api_proto.ApiFlow{}),
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ClientsPlugin{})
	vql_subsystem.RegisterPlugin(&FlowsPlugin{})
}
