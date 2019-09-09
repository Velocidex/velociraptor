// +build server_vql

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
// VQL plugins for running on the server.

package server

import (
	"context"
	"path"

	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/urns"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ClientsPluginArgs struct {
	Search   string `vfilter:"optional,field=search,doc=Client search string. Can have the following prefixes: 'lable:', 'host:'"`
	ClientId string `vfilter:"optional,field=client_id"`
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
		config_obj, ok := any_config_obj.(*config_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		// If a client id is specifies we do not need to search at all.
		if arg.ClientId != "" {
			api_client, err := api.GetApiClient(
				config_obj, nil, arg.ClientId, false)
			if err == nil {
				output_chan <- api_client
			}
			return
		}

		search := arg.Search
		if search == "" {
			search = "all"
		}

		for _, client_id := range db.SearchClients(
			config_obj, constants.CLIENT_INDEX_URN,
			search, "", 0, 1000000) {
			api_client, err := api.GetApiClient(
				config_obj, nil, client_id, false)
			if err == nil {
				output_chan <- api_client
			}
			vfilter.ChargeOp(scope)
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
	FlowId   string   `vfilter:"optional,field=flow_id"`
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
		config_obj, ok := any_config_obj.(*config_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		sender := func(urn string, client_id string) {
			flow_obj, err := flows.GetAFF4FlowObject(config_obj, urn)
			if err != nil {
				return
			}

			if flow_obj.RunnerArgs != nil {
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

		for _, client_id := range arg.ClientId {
			if arg.FlowId != "" {
				urn := urns.BuildURN(
					"clients", client_id, "flows", arg.FlowId)
				sender(urn, client_id)
				continue
			}

			flow_urns, err := db.ListChildren(
				config_obj, urns.BuildURN(
					"clients", client_id, "flows"),
				0, 10000)
			if err != nil {
				return
			}

			for _, urn := range flow_urns {
				sender(urn, client_id)
				vfilter.ChargeOp(scope)
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
