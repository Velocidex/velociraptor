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
package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type AddLabelsArgs struct {
	ClientId string   `vfilter:"required,field=client_id,doc=Client ID to label."`
	Labels   []string `vfilter:"required,field=labels,doc=A list of labels to apply"`
	Op       string   `vfilter:"optional,field=op,doc=An operation on the labels (add, remove)"`
}

type AddLabels struct{}

func (self *AddLabels) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &AddLabelsArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("label: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.LABEL_CLIENT)
	if err != nil {
		scope.Log("label: %s", err)
		return vfilter.Null{}
	}

	any_config_obj, _ := scope.Resolve("server_config")
	config_obj, ok := any_config_obj.(*config_proto.Config)
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
		scope.Log("label: %s", err.Error())
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
