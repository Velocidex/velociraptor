//go:build server_vql
// +build server_vql

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
// VQL plugins for running on the server.

package clients

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ClientsPluginArgs struct {
	Search   string `vfilter:"optional,field=search,doc=Client search string. Can have the following prefixes: 'label:', 'host:'"`
	ClientId string `vfilter:"optional,field=client_id"`
}

type ClientsPlugin struct{}

func (self ClientsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "clients", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("clients: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("clients: %v", err)
			return
		}

		arg := &ClientsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("clients: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("clients: Command can only run on the server")
			return
		}

		// If a client id is specified we do not need to search at all.
		if arg.ClientId != "" {
			indexer, err := services.GetIndexer(config_obj)
			if err != nil {
				scope.Log("clients: %v", err)
				return
			}

			api_client, err := indexer.FastGetApiClient(
				ctx, config_obj, arg.ClientId)
			if err == nil {
				select {
				case <-ctx.Done():
					return
				case output_chan <- json.ConvertProtoToOrderedDict(api_client):
				}
			}
			return
		}

		search_term := arg.Search
		if search_term == "" {
			search_term = "all"
		}

		indexer, err := services.GetIndexer(config_obj)
		if err != nil {
			scope.Log("client_info: %s", err.Error())
			return
		}

		search_chan, err := indexer.SearchClientsChan(ctx, scope,
			config_obj, search_term, vql_subsystem.GetPrincipal(scope))
		if err != nil {
			scope.Log("clients: %v", err)
			return
		}

		for api_client := range search_chan {
			select {
			case <-ctx.Done():
				return
			case output_chan <- json.ConvertProtoToOrderedDict(api_client):
			}
		}
	}()

	return output_chan
}

func (self ClientsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "clients",
		Doc:      "Retrieve the list of clients.",
		ArgType:  type_map.AddType(scope, &ClientsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

type ClientInfoFunctionArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
}

type ClientInfoFunction struct{}

func (self *ClientInfoFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
	if err != nil {
		scope.Log("client_info: %s", err)
		return vfilter.Null{}
	}

	arg := &ClientInfoFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("client_info: %s", err.Error())
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("client_info: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("client_info: Command can only run on the server")
		return vfilter.Null{}
	}

	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		scope.Log("client_info: %s", err.Error())
		return vfilter.Null{}
	}

	api_client, err := indexer.FastGetApiClient(ctx,
		config_obj, arg.ClientId)
	if err != nil {
		scope.Log("client_info: %s", err.Error())
		return vfilter.Null{}
	}
	return json.ConvertProtoToOrderedDict(api_client)
}

func (self ClientInfoFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "client_info",
		Doc:      "Returns client info (like the fqdn) from the datastore.",
		ArgType:  type_map.AddType(scope, &ClientInfoFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
		Version:  2,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ClientInfoFunction{})
	vql_subsystem.RegisterPlugin(&ClientsPlugin{})
}
