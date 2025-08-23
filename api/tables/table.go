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
package tables

import (
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func GetTable(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest,
	principal string) (
	*api_proto.GetTableResponse, error) {

	var result *api_proto.GetTableResponse
	var err error

	// We want an event table.
	switch in.Type {
	case "TIMELINE":
		result, err = getTimeline(ctx, config_obj, in)

	case "CLIENT_EVENT_LOGS", "SERVER_EVENT_LOGS":
		result, err = getEventTableLogs(ctx, config_obj, in)

	case "CLIENT_EVENT", "SERVER_EVENT":
		result, err = getEventTable(ctx, config_obj, in)

	case "STACK":
		result, err = getStackTable(ctx, config_obj, in)

	case "NOTEBOOKS":
		result, err = getNotebookTable(ctx, config_obj, in, principal)

	default:
		result, err = getTable(ctx, config_obj, in, principal)
	}

	if err != nil {
		return nil, err
	}

	if in.Artifact != "" {
		manager, err := services.GetRepositoryManager(config_obj)
		if err != nil {
			return nil, err
		}

		repository, err := manager.GetGlobalRepository(config_obj)
		if err != nil {
			return nil, err
		}

		artifact, pres := repository.Get(ctx, config_obj, in.Artifact)
		if pres {
			result.ColumnTypes = artifact.ColumnTypes
		}
	}

	return result, nil

}

func getTable(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest,
	principal string) (
	*api_proto.GetTableResponse, error) {

	if in.Rows == 0 {
		in.Rows = 2000
	}

	result := &api_proto.GetTableResponse{
		ColumnTypes: getColumnTypes(ctx, config_obj, in),
	}

	path_spec, err := GetPathSpec(ctx, config_obj, in, principal)
	if err != nil {
		return result, err
	}

	file_store_factory := file_store.GetFileStore(config_obj)

	options, err := GetTableOptions(in)
	if err != nil {
		return result, err
	}

	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, config_obj,
		file_store_factory, path_spec, options)

	// if the result does not exist yet, just return an empty result.
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}

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

	stack_path := rs_reader.Stacker()
	if !utils.IsNil(stack_path) {
		result.StackPath = stack_path.Components()
	}

	// Seek to the row we need.
	err = rs_reader.SeekToRow(int64(in.StartRow))
	if errors.Is(err, io.EOF) {
		return result, nil
	}

	if err != nil {
		return nil, err
	}

	return ConvertRowsToTableResponse(
		rs_reader.Rows(ctx), result, in.Timezone, in.Rows), nil
}

func getStackTable(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (
	*api_proto.GetTableResponse, error) {

	if in.Rows == 0 {
		in.Rows = 2000
	}

	result := &api_proto.GetTableResponse{
		ColumnTypes: getColumnTypes(ctx, config_obj, in),
	}

	path_spec := path_specs.NewUnsafeFilestorePath(
		utils.FilterSlice(in.StackPath, "")...).
		SetType(api.PATH_TYPE_FILESTORE_JSON)
	file_store_factory := file_store.GetFileStore(config_obj)

	options, err := GetTableOptions(in)
	if err != nil {
		return result, err
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
	if errors.Is(err, io.EOF) {
		return result, nil
	}

	if err != nil {
		return nil, err
	}

	return ConvertRowsToTableResponse(
		rs_reader.Rows(ctx), result, in.Timezone, in.Rows), nil
}

// The GUI is requesting table data. This function tries to figure out
// the column types.
func getColumnTypes(
	ctx context.Context, config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) []*artifacts_proto.ColumnType {

	// For artifacts column types are specified in the `column_types`
	// artifact definition.
	if in.Artifact != "" {
		manager, err := services.GetRepositoryManager(config_obj)
		if err != nil {
			return nil
		}

		repository, err := manager.GetGlobalRepository(config_obj)
		if err != nil {
			return nil
		}

		artifact, pres := repository.Get(ctx, config_obj, in.Artifact)
		if pres {
			return artifact.ColumnTypes
		}
	}

	// For notebooks, the column_types are set in the notebook metadata.
	if in.NotebookId != "" {
		notebook_manager, err := services.GetNotebookManager(config_obj)
		if err != nil {
			return nil
		}

		notebook, err := notebook_manager.GetNotebook(ctx, in.NotebookId,
			services.DO_NOT_INCLUDE_UPLOADS)
		if err != nil {
			return nil
		}
		return notebook.ColumnTypes
	}

	return nil
}

// Get the relevant pathspec for the table needed. Basically a big
// switch to figure out where the result set we want to look at is
// stored.
func GetPathSpec(
	ctx context.Context, config_obj *config_proto.Config,
	in *api_proto.GetTableRequest, principal string) (api.FSPathSpec, error) {

	if in.Type == "CLIENT_FLOWS" && in.ClientId != "" {
		return paths.NewClientPathManager(in.ClientId).FlowIndex(), nil
	}

	if in.Type == "NOTEBOOKS" {
		return paths.NewNotebookPathManager("").
			NotebookIndexForUser(principal), nil
	}

	if in.FlowId != "" && in.Artifact != "" {
		mode := paths.MODE_CLIENT
		if in.ClientId == "server" {
			mode = paths.MODE_SERVER
		}
		return artifacts.NewArtifactPathManagerWithMode(
			config_obj, in.ClientId, in.FlowId, in.Artifact,
			mode).Path(), nil

	} else if in.FlowId != "" && in.Type != "" {
		flow_path_manager := paths.NewFlowPathManager(
			in.ClientId, in.FlowId)

		switch in.Type {
		case "log":
			return flow_path_manager.Log(), nil

		case "uploads":
			return flow_path_manager.UploadMetadata(), nil

		case "upload_transactions":
			return flow_path_manager.UploadTransactions(), nil
		}

	} else if in.HuntId != "" && in.Type == "clients" {
		return paths.NewHuntPathManager(in.HuntId).Clients(), nil

	} else if in.HuntId != "" && in.Type == "hunt_status" {
		return paths.NewHuntPathManager(in.HuntId).ClientErrors(), nil

	} else if in.NotebookId != "" && in.CellId != "" && in.Type == "logs" {
		return paths.NewNotebookPathManager(in.NotebookId).Cell(
			in.CellId, in.CellVersion).Logs(), nil

		// Handle dashboards specially. Dashboards are kind of
		// non-interactive notebook stored in a special notebook ID
		// called "Dashboards". Cells within the dashboard correspond
		// to different artifacts. Dashboard cells are recalculated
		// each time they are viewed.
	} else if in.NotebookId == "Dashboards" && in.CellId != "" {
		return paths.NewDashboardPathManager(in.Type, in.CellId, in.ClientId).
			QueryStorage(in.TableId).Path(), nil

	} else if in.NotebookId != "" && in.CellId != "" {
		return paths.NewNotebookPathManager(in.NotebookId).Cell(
			in.CellId, in.CellVersion).QueryStorage(in.TableId).Path(), nil
	}

	return nil, errors.New("Invalid request")
}

func getEventTable(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest) (
	*api_proto.GetTableResponse, error) {
	path_manager, err := artifacts.NewArtifactPathManager(ctx,
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
	path_manager, err := artifacts.NewArtifactLogPathManager(ctx,
		config_obj, in.ClientId, "", in.Artifact)
	if err != nil {
		return nil, err
	}
	return getEventTableWithPathManager(ctx, config_obj, in, path_manager)
}

// Unpack the rows into the output protobuf. Although not ideal, each
// row can have a different set of columns that the previous row. We
// keep track of all columns seen in this table page and their
// relative order.
func ConvertRowsToTableResponse(
	in <-chan *ordereddict.Dict,
	result *api_proto.GetTableResponse,
	timezone string,
	limit uint64,
) *api_proto.GetTableResponse {
	opts := json.GetJsonOptsForTimezone(timezone)

	var rows uint64
	column_known := make(map[string]bool)
	for row := range in {
		data := make(map[string]interface{})
		for _, i := range row.Items() {
			// Do we already know about this column?
			_, pres := column_known[i.Key]
			if !pres {
				result.Columns = append(result.Columns, i.Key)
				column_known[i.Key] = true
			}

			data[i.Key] = i.Value
		}

		json_out := make([]interface{}, 0, len(result.Columns))
		for _, k := range result.Columns {
			value, _ := data[k]
			json_out = append(json_out, value)
		}
		serialized, err := json.MarshalWithOptions(json_out, opts)
		if err != nil {
			continue
		}

		result.Rows = append(result.Rows, &api_proto.Row{
			Json: string(serialized),
		})

		rows += 1
		if rows >= limit {
			break
		}
	}

	return result
}

func getEventTableWithPathManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.GetTableRequest,
	path_manager api.PathManager) (
	*api_proto.GetTableResponse, error) {

	if in.Rows == 0 {
		in.Rows = 10
	}

	result := &api_proto.GetTableResponse{}

	rs_reader, err := result_sets.NewTimedResultSetReader(ctx,
		config_obj, path_manager)
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

	return ConvertRowsToTableResponse(
		rs_reader.Rows(ctx), result, in.Timezone, in.Rows), nil
}

func GetTableOptions(in *api_proto.GetTableRequest) (
	options result_sets.ResultSetOptions, err error) {
	if in.SortColumn != "" {
		options.SortColumn = in.SortColumn
		options.SortAsc = in.SortDirection
	}

	if in.FilterColumn != "" &&
		in.FilterRegex != "" {
		options.FilterColumn = in.FilterColumn

		// If the filter has a ! in the first position it excludes the
		// match.
		if strings.HasPrefix(in.FilterRegex, "!") {
			in.FilterRegex = in.FilterRegex[1:]
			options.FilterExclude = true
		}

		options.FilterRegex, err = regexp.Compile("(?i)" + in.FilterRegex)
		if err != nil {
			return options, err
		}
	}

	options.StartIdx = in.StartIdx
	options.EndIdx = in.EndIdx

	return options, nil
}
