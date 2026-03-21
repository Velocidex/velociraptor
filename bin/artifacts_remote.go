package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	artifact_command_fetch = artifact_command.Command(
		"fetch", "Fetch a collection from the server via the API")

	artifact_command_fetch_client_id = artifact_command_fetch.Flag(
		"client_id", "Used for remote API calls to specify the client id "+
			"to collect from. By default this is `server` to server "+
			"artifacts").
		Default("server").String()

	artifact_command_fetch_flow_id = artifact_command_fetch.Flag(
		"flow_id", "Used for remote API calls to specify the client id "+
			"to collect from. By default this is `server` to server "+
			"artifacts").Required().String()

	artifact_command_fetch_org_id = artifact_command_fetch.Flag(
		"org_id", "Used for remote API calls to specify the org id "+
			"(default `root`)").
		Default("root").String()

	artifact_command_fetch_output = artifact_command_fetch.Flag(
		"output", "The path to create a zip file to store the collection into.").
		Required().String()
)

// Handler for `artifacts fetch`
func doArtifactFetch() error {
	logging.DisableLogging()

	org_id := *artifact_command_fetch_org_id
	output := *artifact_command_fetch_output

	config_obj, err := APIConfigLoader.WithNullLoader().
		LoadAndValidate()
	if err != nil {
		return err
	}

	ctx, top_cancel := install_sig_handler()
	defer top_cancel()

	if config_obj.ApiConfig == nil || config_obj.ApiConfig.Name == "" {
		return errors.New("The `artifacts fetch` command requires an API config")
	}

	request := &actions_proto.VQLCollectorArgs{
		OrgId:   org_id,
		MaxRow:  1000,
		MaxWait: 1,
		Env: []*actions_proto.VQLEnv{
			{
				Key:   "FlowId",
				Value: *artifact_command_fetch_flow_id,
			},
			{
				Key:   "ClientId",
				Value: *artifact_command_fetch_client_id,
			},
		},
		Query: []*actions_proto.VQLRequest{
			{
				VQL: `
SELECT create_flow_download(
   client_id=ClientId, flow_id=FlowId, wait=TRUE).Components AS Download
FROM scope()
`,
			},
		},
	}

	return callAPIWithRequest(ctx, config_obj, request, output)
}

func doRemoteArtifactCollection(
	ctx context.Context,
	config_obj *config_proto.Config,
	spec *ordereddict.Dict,
	org_id string,
	cpu_limit, timeout int64,
	client_id string,
	output string,
) error {

	var artifacts []string
	for _, item := range spec.Items() {
		artifacts = append(artifacts, item.Key)
	}

	request := &actions_proto.VQLCollectorArgs{
		OrgId:    org_id,
		MaxRow:   1000,
		MaxWait:  1,
		CpuLimit: float32(cpu_limit),
		Timeout:  uint64(timeout),
		Env: []*actions_proto.VQLEnv{
			{
				Key:   "Artifacts",
				Value: json.MustMarshalString(artifacts),
			},
			{
				Key:   "Spec",
				Value: json.MustMarshalString(spec),
			},
			{
				Key:   "ClientId",
				Value: client_id,
			},
		},
		Query: []*actions_proto.VQLRequest{
			{
				VQL: `
LET Flow <= collect_client(client_id=ClientId,
   artifacts=parse_json_array(data=Artifacts),
   spec=parse_json(data=Spec))
`,
			},
			{
				VQL: `
LET _ <= log(message="<green>Scheduled</> flow %v on client %v: %v",
   args=[Flow.flow_id, ClientId, Flow.request.artifacts])
`,
			},

			// Wait here for the flow to complete.
			{
				VQL: `
LET ResultFlow <= SELECT *, "FlowInfo" AS _Source
FROM foreach(row={
   SELECT * FROM clock(period=1)
}, query={
   SELECT * FROM flows(client_id=ClientId, flow_id=Flow.flow_id)
   WHERE log(message="Waiting for flow to finish %v: Status %v",
             args=[Flow.flow_id, state], dedup=5)
     AND state =~ "FINISHED|ERROR"
})
LIMIT 1
`,
			},
			{
				VQL: `
LET _ <= if(condition=ResultFlow.state =~ "ERROR",
  then=log(message="<red>Collection failed:</> %v", level="ERROR",
           args=ResultFlow[0].status),
  else=log(message="<green>Collection succeeded</>"))
`,
			},
		},
	}

	if output == "" {
		// Dump the output to stdout
		request.Query = append(request.Query,
			&actions_proto.VQLRequest{
				VQL: `
SELECT * FROM foreach(
row=get_flow(client_id=ClientId, flow_id=Flow.flow_id).artifacts_with_results,
query={
   SELECT *, _value AS _Source
   FROM flow_results(client_id=ClientId, flow_id=Flow.flow_id, artifact=_value)
})
`,
			})
	} else {
		// Prepare a flow download of the results.
		request.Query = append(request.Query,
			&actions_proto.VQLRequest{
				VQL: `
SELECT create_flow_download(client_id=ClientId, flow_id=Flow.flow_id, wait=TRUE).Components AS Download
FROM scope()
`,
			})
	}

	return callAPIWithRequest(ctx, config_obj, request, output)
}

func callAPIWithRequest(ctx context.Context,
	config_obj *config_proto.Config,
	request *actions_proto.VQLCollectorArgs,
	output string) error {
	// Make a remote query using the API - we better have user API
	// credentials in the config file.
	client, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, grpc_client.API_User, config_obj)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	logger := &LogWriter{
		config_obj: config_obj,
	}

	stream, err := client.Query(ctx, request)
	if err != nil {
		return err
	}

	for {
		response, err := stream.Recv()
		if response == nil && err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if response.Log != "" {
			logger.Write([]byte(response.Log))
			continue
		}

		json_response := response.Response
		if json_response == "" {
			json_response = response.JSONLResponse
		}

		rows, err := utils.ParseJsonToDicts([]byte(json_response))
		if err != nil {
			return err
		}

		for _, row := range rows {
			fmt.Println(json.MustMarshalString(row))
			download, pres := row.GetStrings("Download")
			if pres && output != "" {
				err := fetchFile(ctx, config_obj, client,
					download, output, request.OrgId)
				if err != nil {
					return err
				}
			}
		}
	}

	return logger.Error
}

func fetchFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	client api_proto.APIClient,
	download []string,
	output string,
	org_id string) error {

	fd, err := os.OpenFile(output,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()

	h := sha256.New()
	defer func() {
		logging.GetLogger(config_obj, &logging.ClientComponent).Info(
			"<green>Storing collection in </>%s with SHA256 hash %x",
			output, h.Sum(nil))
	}()

	blocksize := uint32(1024 * 1024)
	offset := uint64(0)

	for {
		request := &api_proto.VFSFileBuffer{
			OrgId:      org_id,
			Components: download,
			Length:     blocksize,
			Offset:     offset,
		}

		response, err := client.VFSGetBuffer(ctx, request)
		if err != nil {
			return err
		}

		if len(response.Data) == 0 {
			break
		}

		_, err = fd.Write(response.Data)
		if err != nil {
			return err
		}

		h.Write(response.Data)

		offset += uint64(len(response.Data))
	}

	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case artifact_command_fetch.FullCommand():
			FatalIfError(artifact_command_fetch, doArtifactFetch)

		default:
			return false
		}
		return true
	})
}
