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
	"regexp"
	"time"

	errors "github.com/pkg/errors"
	context "golang.org/x/net/context"
	"www.velocidex.com/golang/velociraptor/datastore"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
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

	result := &api_proto.GetTableResponse{
		ColumnTypes: getColumnTypes(config_obj, in),
	}

	path_spec, err := getPathSpec(config_obj, in)
	if err != nil {
		return result, err
	}

	file_store_factory := file_store.GetFileStore(config_obj)

	options := result_sets.ResultSetOptions{}
	if in.SortColumn != "" {
		options.SortColumn = in.SortColumn
		options.SortAsc = in.SortDirection
	}

	if in.FilterColumn != "" &&
		in.FilterRegex != "" {
		options.FilterColumn = in.FilterColumn
		options.FilterRegex, err = regexp.Compile("(?i)" + in.FilterRegex)
		if err != nil {
			return nil, err
		}
	}

	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, config_obj,
		file_store_factory, path_spec, options)

	if err != nil {
		return result, nil
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

// The GUI is requesting table data. This function tries to figure out
// the column types.
func getColumnTypes(
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) []*artifacts_proto.ColumnType {

	// For artifacts column types are specified in the `column_types`
	// artifact definition.
	if in.Artifact != "" {
		manager, err := services.GetRepositoryManager()
		if err != nil {
			return nil
		}

		repository, err := manager.GetGlobalRepository(config_obj)
		if err != nil {
			return nil
		}

		artifact, pres := repository.Get(config_obj, in.Artifact)
		if pres {
			return artifact.ColumnTypes
		}
	}

	// For notebooks, the column_types are set in the notebook metadata.
	if in.NotebookId != "" {
		notebook_path_manager := paths.NewNotebookPathManager(
			in.NotebookId)

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return nil
		}

		notebook := &api_proto.NotebookMetadata{}
		err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
		if err == nil {
			return notebook.ColumnTypes
		}
	}

	return nil
}

func getPathSpec(
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (api.FSPathSpec, error) {

	if in.FlowId != "" && in.Artifact != "" {
		path_manager, err := artifacts.NewArtifactPathManager(
			config_obj, in.ClientId, in.FlowId, in.Artifact)
		if err != nil {
			return nil, err
		}
		return path_manager.Path(), nil

	} else if in.FlowId != "" && in.Type != "" {
		flow_path_manager := paths.NewFlowPathManager(
			in.ClientId, in.FlowId)

		switch in.Type {
		case "log":
			// Handle legacy locations. TODO: Remove by 0.6.7
			file_store_factory := file_store.GetFileStore(config_obj)
			pathspec := flow_path_manager.Log()
			_, err := file_store_factory.StatFile(pathspec)
			if err != nil {
				utils.Debug(flow_path_manager.LogLegacy())
				return flow_path_manager.LogLegacy(), nil
			}
			return pathspec, nil

		case "uploads":
			return flow_path_manager.UploadMetadata(), nil
		}
	} else if in.HuntId != "" && in.Type == "clients" {
		return paths.NewHuntPathManager(in.HuntId).Clients(), nil

	} else if in.HuntId != "" && in.Type == "hunt_status" {
		return paths.NewHuntPathManager(in.HuntId).ClientErrors(), nil

	} else if in.NotebookId != "" && in.CellId != "" {
		return paths.NewNotebookPathManager(in.NotebookId).Cell(
			in.CellId).QueryStorage(in.TableId).Path(), nil
	}

	return nil, errors.New("Invalid request")
}

func getEventTable(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (
	*api_proto.GetTableResponse, error) {
	path_manager, err := artifacts.NewArtifactPathManager(
		config_obj, in.ClientId, in.FlowId, in.Artifact)
	if err != nil {
		return nil, err
	}

	return getEventTableWithPathManager(ctx, config_obj, in, path_manager)
}

func getEventTableLogs(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (
	*api_proto.GetTableResponse, error) {
	path_manager, err := artifacts.NewArtifactLogPathManager(
		config_obj, in.ClientId, "", in.Artifact)
	if err != nil {
		return nil, err
	}
	return getEventTableWithPathManager(ctx, config_obj, in, path_manager)
}

func getEventTableWithPathManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest,
	path_manager api.PathManager) (
	*api_proto.GetTableResponse, error) {

	rows := uint64(0)
	if in.Rows == 0 {
		in.Rows = 10
	}

	result := &api_proto.GetTableResponse{}

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewTimedResultSetReader(ctx,
		file_store_factory, path_manager)
	if err != nil {
		return nil, err
	}
	defer rs_reader.Close()

	err = rs_reader.SeekToTime(time.Unix(int64(in.StartTime), 0))
	if err != nil {
		return nil, err
	}

	if in.EndTime != 0 {
		rs_reader.SetMaxTime(time.Unix(int64(in.EndTime), 0))
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

func getTimeline(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	if in.NotebookId == "" {
		return nil, errors.New("NotebookId must be specified")
	}

	path_manager := paths.NewNotebookPathManager(in.NotebookId).
		SuperTimeline(in.Timeline)
	reader, err := timelines.NewSuperTimelineReader(
		config_obj, path_manager, in.SkipComponents)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	result := &api_proto.GetTableResponse{
		Columns:   []string{"_Source", "Time", "Data"},
		StartTime: int64(in.StartTime),
	}

	if in.StartTime != 0 {
		ts := time.Unix(0, int64(in.StartTime))
		reader.SeekToTime(ts)
	}

	rows := uint64(0)
	for item := range reader.Read(ctx) {
		if result.StartTime == 0 {
			result.StartTime = item.Time.UnixNano()
		}
		result.EndTime = item.Time.UnixNano()
		result.Rows = append(result.Rows, &api_proto.Row{
			Cell: []string{
				item.Source,
				csv.AnyToString(item.Time),
				csv.AnyToString(item.Row)},
		})

		rows += 1
		if rows > in.Rows {
			break
		}
	}

	return result, nil
}
