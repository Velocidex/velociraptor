package hunt_dispatcher

import (
	"context"
	"errors"
	"io"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

func (self *HuntDispatcher) syncFlowTables(
	ctx context.Context, config_obj *config_proto.Config,
	hunt_id string) error {

	count := 0

	now := utils.GetTime().Now()

	options := result_sets.ResultSetOptions{}
	scope := vql_subsystem.MakeScope()
	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, self.config_obj, file_store_factory,
		hunt_path_manager.Clients(), options)
	if err != nil {
		return err
	}
	defer rs_reader.Close()

	enriched_reader, err := result_sets.NewResultSetReaderWithOptions(
		ctx, self.config_obj, file_store_factory,
		hunt_path_manager.EnrichedClients(), options)
	if err == nil {
		enriched_reader.Close()

		// Skip refreshing the enriched table if it is newer than 5 min
		// old - this helps to reduce unnecessary updates.
		if now.Sub(enriched_reader.MTime()) < 5*time.Minute {
			return nil
		}
	}

	// Report how long it took to refresh the table
	defer func() {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("<green>HuntDispatcher:</> Mirrored client table in %v (%v records)",
			utils.GetTime().Now().Sub(now), count)
	}()

	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		hunt_path_manager.EnrichedClients(), json.DefaultEncOpts(),
		utils.SyncCompleter, result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	for row := range rs_reader.Rows(ctx) {
		participation_row := &hunt_manager.ParticipationRecord{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, row, participation_row)
		if err != nil {
			return err
		}

		flow, err := launcher.GetFlowDetails(
			ctx, config_obj,
			services.GetFlowOptions{},
			participation_row.ClientId, participation_row.FlowId)
		if err != nil {
			continue
		}

		count++
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
	return nil
}

func (self *HuntDispatcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	options services.FlowSearchOptions, scope vfilter.Scope,
	hunt_id string, start int) (chan *api_proto.FlowDetails, int64, error) {

	output_chan := make(chan *api_proto.FlowDetails)

	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	table_to_query := hunt_path_manager.Clients()

	// We only need to sync the tables if the options need to use
	// anything other than the default table, otherwise we just query
	// the original table.
	if options.SortColumn != "" || options.FilterColumn != "" {
		err := self.syncFlowTables(ctx, config_obj, hunt_id)
		if err != nil {
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

	launcher, err := services.GetLauncher(config_obj)
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
