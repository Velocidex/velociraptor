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
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/result_sets"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func getTable(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (
	*api_proto.GetTableResponse, error) {

	rows := uint64(0)
	if in.Rows == 0 {
		in.Rows = 500
	}

	var path_manager api.PathManager

	if in.Type == "inventory" {
		path_manager = paths.NewInventoryPathManager()

	} else if in.FlowId != "" && in.Artifact != "" {
		path_manager = result_sets.NewArtifactPathManager(
			config_obj, in.ClientId, in.FlowId, in.Artifact)

	} else if in.FlowId != "" && in.Type != "" {
		flow_path_manager := paths.NewFlowPathManager(
			in.ClientId, in.FlowId)
		switch in.Type {
		case "log":
			path_manager = flow_path_manager.Log()
		case "uploads":
			path_manager = flow_path_manager.UploadMetadata()
		}
	} else if in.HuntId != "" && in.Type == "clients" {
		path_manager = paths.NewHuntPathManager(in.HuntId).Clients()

	} else if in.HuntId != "" && in.Type == "hunt_status" {
		path_manager = paths.NewHuntPathManager(in.HuntId).ClientErrors()

	} else if in.NotebookId != "" && in.CellId != "" {
		path_manager = reporting.NewNotebookPathManager(in.NotebookId).Cell(
			in.CellId).QueryStorage(in.TableId)
	}

	result := &api_proto.GetTableResponse{}

	if path_manager != nil {
		row_chan, err := file_store.GetTimeRange(ctx, config_obj,
			path_manager, 0, 0)
		if err != nil {
			return nil, err
		}

		for row := range row_chan {
			if result.Columns == nil {
				result.Columns = row.Keys()
			}

			row_data := make([]string, 0, len(result.Columns))
			for _, key := range row.Keys() {
				value, _ := row.Get(key)
				row_data = append(row_data, csv.AnyToString(value))
			}
			result.Rows = append(result.Rows, &api_proto.Row{
				Cell: row_data,
			})

			rows += 1
			if rows > in.Rows {
				break
			}
		}
	}

	return result, nil
}
