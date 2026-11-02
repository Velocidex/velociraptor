package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func doRemoteQuery(
	config_obj *config_proto.Config, format string, org_id string,
	queries []string, env *ordereddict.Dict) error {

	logging.DisableLogging()

	ctx, cancel := Install_sig_handler()
	defer cancel()

	// Make a remote query using the API - we better have user API
	// credentials in the config file.
	client, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, grpc_client.API_User, config_obj)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)

	request := &actions_proto.VQLCollectorArgs{
		OrgId:    org_id,
		MaxRow:   1000,
		MaxWait:  1,
		CpuLimit: float32(*query_command_collect_cpu_limit),
		Timeout:  uint64(*query_command_collect_timeout),
	}

	if env != nil {
		for _, i := range env.Items() {
			request.Env = append(request.Env, &actions_proto.VQLEnv{
				Key: i.Key, Value: utils.ToString(i.Value)})
		}
	}

	for _, query := range queries {
		request.Query = append(request.Query,
			&actions_proto.VQLRequest{VQL: query})
	}
	stream, err := client.Query(context.Background(), request)
	if err != nil {
		return err
	}

	for {
		response, err := stream.Recv()
		if err != nil && err != io.EOF {
			return err
		}

		if response == nil {
			break
		}

		if response.Log != "" {
			logger.Info("%s", response.Log)
			continue
		}

		// Response can be line at a time, or a chunk of rows.
		json_response := response.Response
		if json_response == "" {
			json_response = response.JSONLResponse
		}

		rows, err := utils.ParseJsonToDicts([]byte(json_response))
		if err != nil {
			return err
		}

		switch format {
		case "text":
			vfilter_rows := make([]vfilter.Row, 0, len(rows))
			for _, row := range rows {
				vfilter_rows = append(vfilter_rows, row)
			}

			scope := vql_subsystem.MakeScope()
			table := reporting.OutputRowsToTable(scope, vfilter_rows, os.Stdout)
			table.Render()

		case "json":
			fmt.Println(string(json.MustMarshalIndent(rows)))

		case "jsonl":
			for _, row := range rows {
				fmt.Println(json.MustMarshalString(row))
			}

		case "csv":
			scope := vql_subsystem.MakeScope()

			csv_writer := csv.GetCSVAppender(config_obj,
				scope, &StdoutWrapper{os.Stdout},
				csv.WriteHeaders, json.DefaultEncOpts())
			defer csv_writer.Close()

			for _, row := range rows {
				csv_writer.Write(row)
			}
		}
	}
	return nil
}
