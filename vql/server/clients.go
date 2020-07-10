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

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
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
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("clients: %v", err)
			return
		}

		arg := &ClientsPluginArgs{}
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("clients: %v", err)
			return
		}

		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		// If a client id is specified we do not need to search at all.
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
		ArgType: type_map.AddType(scope, &ClientsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ClientsPlugin{})
}
