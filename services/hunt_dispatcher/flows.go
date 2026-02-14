package hunt_dispatcher

import (
	"context"
	"errors"
	"io"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func syncFlowTables(
	ctx context.Context,
	config_obj *config_proto.Config,
	launcher services.Launcher,
	hunt_id string,
	refresh_stats *HuntRefreshStats) (*api_proto.HuntStats, error) {

	// Update the stats if needed.
	stats := &api_proto.HuntStats{}
	now := utils.GetTime().Now()

	options := result_sets.ResultSetOptions{}
	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, config_obj, file_store_factory,
		hunt_path_manager.Clients(), options)
	if err != nil {
		return nil, err
	}
	defer rs_reader.Close()

	enriched_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, config_obj, file_store_factory,
		hunt_path_manager.EnrichedClients(), options)
	if err == nil {
		enriched_reader.Close()

		// Skip refreshing the enriched table if it is newer than 5 min
		// old - this helps to reduce unnecessary updates.
		if now.Sub(enriched_reader.MTime()) < HuntDispatcherRefreshSec(config_obj) {
			return nil, utils.CancelledError
		}
	}

	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		hunt_path_manager.EnrichedClients(), json.DefaultEncOpts(),
		utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}
	defer rs_writer.Close()

	json_chan, err := rs_reader.JSON(ctx)
	if err != nil {
		return nil, err
	}

	for json_str := range json_chan {
		participation_row := &hunt_manager.ParticipationRecord{}
		err := json.Unmarshal(json_str, participation_row)
		if err != nil {
			continue
		}

		refresh_stats.TotalFlowsInspected++

		// If the client is deleted or the flow disappeared, this will
		// error out. We then ignore this row.
		flow, err := launcher.GetFlowDetails(
			ctx, config_obj,
			services.GetFlowOptions{},
			participation_row.ClientId, participation_row.FlowId)
		if err != nil || flow.Context == nil {
			continue
		}

		stats.TotalClientsScheduled++
		stats.TotalUploadedBytes += flow.Context.TotalUploadedBytes
		stats.TotalCollectedRows += flow.Context.TotalCollectedRows
		if flow.Context.TotalCollectedRows > 0 {
			stats.TotalClientsWithResults++
		}

		switch flow.Context.State {
		case flows_proto.ArtifactCollectorContext_ERROR:
			stats.TotalClientsWithErrors++
			stats.TotalFinishedClients++

		case flows_proto.ArtifactCollectorContext_FINISHED:
			stats.TotalFinishedClients++
		}

		rs_writer.WriteJSONL([]byte(
			json.Format(`{"ClientId": %q, "Hostname": %q, "FlowId": %q, "StartedTime": %q, "State": %q, "Duration": %q, "TotalBytes": %q, "TotalRows": %q}
`,
				participation_row.ClientId,
				services.GetHostname(ctx, config_obj, participation_row.ClientId),
				participation_row.FlowId,
				flow.Context.StartTime/1000,
				flow.Context.State.String(),
				flow.Context.ExecutionDuration/1000000000,
				flow.Context.TotalUploadedBytes,
				flow.Context.TotalCollectedRows)), 1)
	}
	return stats, nil
}

func (self *HuntDispatcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	options services.FlowSearchOptions, scope vfilter.Scope,
	hunt_id string, start int) (chan *api_proto.FlowDetails, int64, error) {

	output_chan := make(chan *api_proto.FlowDetails)

	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	table_to_query := hunt_path_manager.Clients()

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		close(output_chan)
		return output_chan, 0, err
	}

	// We only need to sync the tables if the options need to use
	// anything other than the default table, otherwise we just query
	// the original table.
	if options.SortColumn != "" || options.FilterColumn != "" {
		_, err := syncFlowTables(ctx, config_obj, launcher, hunt_id,
			&HuntRefreshStats{})
		if err != nil && !errors.Is(err, utils.CancelledError) {
			close(output_chan)
			return output_chan, 0, err
		}
		table_to_query = hunt_path_manager.EnrichedClients()
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, self.config_obj, file_store_factory,
		table_to_query, options.ResultSetOptions)
	if err != nil {
		close(output_chan)
		return output_chan, 0, err
	}

	// Seek to the row we need.
	err = rs_reader.SeekToRow(int64(start))
	if errors.Is(err, io.EOF) {
		close(output_chan)
		rs_reader.Close()

		return output_chan, 0, nil
	}

	if err != nil {
		close(output_chan)
		rs_reader.Close()
		return output_chan, 0, err
	}

	go func() {
		defer close(output_chan)
		defer rs_reader.Close()

		for row := range rs_reader.Rows(ctx) {
			client_id, pres := row.GetString("ClientId")
			if !pres {
				client_id, pres = row.GetString("client_id")
				if !pres {
					continue
				}
			}

			flow_id, pres := row.GetString("FlowId")
			if !pres {
				flow_id, pres = row.GetString("flow_id")
				if !pres {
					continue
				}
			}

			var collection_context *api_proto.FlowDetails

			if options.BasicInformation {
				collection_context = &api_proto.FlowDetails{
					Context: &flows_proto.ArtifactCollectorContext{
						ClientId:  client_id,
						SessionId: flow_id,
					},
				}

				// If the user wants detailed flow information we need
				// to fetch this now. For many uses this is not
				// necessary so we can get away with very basic
				// information.
			} else {
				collection_context, err = launcher.GetFlowDetails(
					ctx, config_obj, services.GetFlowOptions{},
					client_id, flow_id)
				if err != nil {
					continue
				}
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- collection_context:
			}
		}
	}()

	return output_chan, rs_reader.TotalRows(), nil
}
