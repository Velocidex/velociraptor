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
package api

import (
	"context"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	vjson "www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

func RunVQL(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	env *ordereddict.Dict,
	query string) (*api_proto.GetTableResponse, error) {

	result := &api_proto.GetTableResponse{}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}
	scope := manager.BuildScope(services.ScopeBuilder{
		Config:     config_obj,
		Env:        env,
		ACLManager: acl_managers.NewServerACLManager(config_obj, principal),
		Logger:     logging.NewPlainLogger(config_obj, &logging.ToolComponent),
	})
	defer scope.Close()

	vql, err := vfilter.Parse(query)
	if err != nil {
		return nil, err
	}

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for row := range vql.Eval(sub_ctx, scope) {
		if len(result.Columns) == 0 {
			result.Columns = scope.GetMembers(row)
		}

		new_row := make([]interface{}, 0, len(result.Columns))
		for _, column := range result.Columns {
			value, pres := scope.Associative(row, column)
			if !pres {
				value = ""
			}
			new_row = append(new_row, value)
		}

		opts := vjson.DefaultEncOpts()
		serialized, err := json.MarshalWithOptions(new_row, opts)
		if err != nil {
			continue
		}
		result.Rows = append(result.Rows, &api_proto.Row{
			Json: string(serialized),
		})
	}

	return result, nil
}
