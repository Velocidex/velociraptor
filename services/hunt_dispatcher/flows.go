package hunt_dispatcher

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_manager"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

func (self *HuntDispatcher) GetFlows(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	hunt_id string, start int) chan *api_proto.FlowDetails {
	output_chan := make(chan *api_proto.FlowDetails)

	go func() {
		defer close(output_chan)

		hunt_path_manager := paths.NewHuntPathManager(hunt_id).Clients()
		file_store_factory := file_store.GetFileStore(config_obj)
		rs_reader, err := result_sets.NewResultSetReader(
			file_store_factory, hunt_path_manager)
		if err != nil {
			scope.Log("hunt_flows: %v\n", err)
			return
		}
		defer rs_reader.Close()

		// Seek to the row we need.
		err = rs_reader.SeekToRow(int64(start))
		if err != nil {
			scope.Log("hunt_flows: %v\n", err)
			return
		}

		launcher, err := services.GetLauncher()
		if err != nil {
			scope.Log("hunt_flows: %v\n", err)
			return
		}

		for row := range rs_reader.Rows(ctx) {
			participation_row := &hunt_manager.ParticipationRecord{}
			err := arg_parser.ExtractArgsWithContext(ctx, scope, row, participation_row)
			if err != nil {
				return
			}

			collection_context, err := launcher.GetFlowDetails(
				config_obj, participation_row.ClientId,
				participation_row.FlowId)
			if err != nil {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- collection_context:
			}
		}
	}()

	return output_chan
}
