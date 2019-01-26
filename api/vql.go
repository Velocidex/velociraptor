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
package api

import (
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/vfilter"
)

func RunVQL(
	ctx context.Context,
	config_obj *api_proto.Config,
	env *vfilter.Dict,
	query string) (*api_proto.GetTableResponse, error) {

	result := &api_proto.GetTableResponse{}

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	env.Set("server_config", config_obj)

	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = logging.NewPlainLogger(config_obj,
		&logging.ToolComponent)

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

		new_row := &api_proto.Row{}
		for _, column := range result.Columns {
			value, pres := scope.Associative(row, column)
			if !pres {
				value = ""
			}
			new_row.Cell = append(new_row.Cell, csv.AnyToString(value))
		}

		result.Rows = append(result.Rows, new_row)
	}

	return result, nil
}
