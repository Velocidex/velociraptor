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
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/reporting"

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

	result := &api_proto.GetTableResponse{}

	path_manager := getPathManager(config_obj, in)
	if path_manager == nil {
		return result, nil
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager)
	if err != nil {
		return nil, err
	}
	defer rs_reader.Close()

	// Let the browser know how many rows we have in total.
	result.TotalRows = rs_reader.TotalRows()

	// FIXME: Backwards compatibility: Just give a few
	// rows if the result set does not have an index. This
	// is the same as the previous behavior but for new
	// collections, an index is created and we respect the
	// number of rows the callers asked for. Eventually
	// this will not be needed.
	if result.TotalRows < 0 {
		in.Rows = 100
	}

	// Seek to the row we need.
	err = rs_reader.SeekToRow(int64(in.StartRow))
	if err != nil {
		return nil, err
	}

	// Unpack the rows into the output protobuf
	for row := range rs_reader.Rows(ctx) {
		if result.Columns == nil {
			result.Columns = row.Keys()
		}

		row_data := make([]string, 0, len(result.Columns))
		for _, key := range result.Columns {
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

	return result, nil
}

func getPathManager(
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) api.PathManager {
	if in.FlowId != "" && in.Artifact != "" {
		return artifacts.NewArtifactPathManager(
			config_obj, in.ClientId, in.FlowId, in.Artifact)

	} else if in.FlowId != "" && in.Type != "" {
		flow_path_manager := paths.NewFlowPathManager(
			in.ClientId, in.FlowId)
		switch in.Type {
		case "log":
			return flow_path_manager.Log()
		case "uploads":
			return flow_path_manager.UploadMetadata()
		}
	} else if in.HuntId != "" && in.Type == "clients" {
		return paths.NewHuntPathManager(in.HuntId).Clients()

	} else if in.HuntId != "" && in.Type == "hunt_status" {
		return paths.NewHuntPathManager(in.HuntId).ClientErrors()

	} else if in.NotebookId != "" && in.CellId != "" {
		return reporting.NewNotebookPathManager(in.NotebookId).Cell(
			in.CellId).QueryStorage(in.TableId)
	}

	return nil
}
