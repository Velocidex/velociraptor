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
package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type AddLabelsArgs struct {
	ClientId string   `vfilter:"required,field=client_id,doc=Client ID to label."`
	Labels   []string `vfilter:"required,field=labels,doc=A list of labels to apply"`
	Op       string   `vfilter:"optional,field=op,doc=An operation on the labels (set, check, remove)"`
}

type AddLabels struct{}

func (self *AddLabels) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &AddLabelsArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("label: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.LABEL_CLIENT)
	if err != nil {
		scope.Log("label: %s", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("label: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("label: Command can only run on the server")
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	labeler := services.GetLabeler(config_obj)
	for _, label := range arg.Labels {
		if label == "" {
			continue
		}

		switch arg.Op {
		case "set":
			err = labeler.SetClientLabel(ctx, config_obj, arg.ClientId, label)
			if err == nil {
				err := services.LogAudit(ctx,
					config_obj, principal, "SetClientLabel",
					ordereddict.NewDict().
						Set("client_id", arg.ClientId).
						Set("label", label))
				if err != nil {
					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Error("<red>SetClientLabel</> %v %v %v",
						principal, arg.ClientId, label)
				}

			}

		case "remove":
			err = labeler.RemoveClientLabel(
				ctx, config_obj, arg.ClientId, label)
			if err == nil {
				err := services.LogAudit(ctx,
					config_obj, principal, "RemoveClientLabel",
					ordereddict.NewDict().
						Set("client_id", arg.ClientId).
						Set("label", label))
				if err != nil {
					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Error("<red>RemoveClientLabel</> %v %v", principal, arg.ClientId)
				}

			}

		case "check":
			if !labeler.IsLabelSet(ctx, config_obj, arg.ClientId, label) {
				return false
			}
		}
		if err != nil {
			scope.Log("label: %s", err.Error())
			return vfilter.Null{}
		}
	}
	return vfilter.RowToDict(ctx, scope, arg)
}

func (self AddLabels) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "label",
		Doc: "Add the labels to the client. " +
			"If op is 'remove' then remove these labels.",
		ArgType:  type_map.AddType(scope, &AddLabelsArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.LABEL_CLIENT).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AddLabels{})
}
