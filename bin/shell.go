// +build !aix

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
package main

import (
	"context"
	"fmt"
	"time"

	prompt "github.com/c-bata/go-prompt"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	// Command line interface for VQL commands.
	shell        = app.Command("shell", "Run an interactive shell on a client")
	shell_client = shell.Arg("client_id", "The client id to run the shell for.").
			Required().String()

	shell_artifact = shell.Flag("type", "The type of shell to run (cmd, powershell, bash)").
			Default("cmd").Enum("cmd", "powershell", "bash")

	shell_alt_artifact = shell.Flag("artifact", "An alternative artifact to run").String()
)

func shell_executor(config_obj *config_proto.Config,
	ctx context.Context,
	client_id string,
	t string) {

	if t == "" {
		return
	}

	artifact_name := "Windows.System.CmdShell"
	switch *shell_artifact {
	case "bash":
		artifact_name = "Linux.Sys.BashShell"
	case "cmd":
		artifact_name = "Windows.System.CmdShell"
	case "powershell":
		artifact_name = "Windows.System.PowerShell"
	}

	if *shell_alt_artifact != "" {
		artifact_name = *shell_alt_artifact
	}

	fmt.Printf("Running %v on %v\n", t, client_id)
	client, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	defer closer()

	response, err := client.CollectArtifact(ctx,
		api.MakeCollectorRequest(client_id, artifact_name, "Command", t))
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	// Wait here until the flow is completed.
	flow_id := response.FlowId
	for {
		response, err := client.GetFlowDetails(ctx, &api_proto.ApiFlowRequest{
			ClientId: client_id, FlowId: flow_id})
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			return
		}

		if response.Context.State == flows_proto.ArtifactCollectorContext_ERROR {
			fmt.Printf("ERROR: %v\n", response.Context.Status)
			return
		}

		if response.Context.State == flows_proto.ArtifactCollectorContext_TERMINATED {
			request := &api_proto.GetTableRequest{
				FlowId:   flow_id,
				Artifact: artifact_name,
				ClientId: client_id,
			}
			response, err := client.GetTable(ctx, request)
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
				return
			}

			for _, row := range response.Rows {
				fmt.Println(row.Cell[0])

				stderr := row.Cell[1]
				if stderr != "" {
					fmt.Println("STDERR: ")
					fmt.Println(stderr)
				}
			}
			return
		}

		time.Sleep(1)
	}
}

func completer(t prompt.Document) []prompt.Suggest {
	return []prompt.Suggest{}
}

func getClientInfo(config_obj *config_proto.Config, ctx context.Context) (*api_proto.ApiClient, error) {
	client, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	if err != nil {
		return nil, err
	}
	defer closer()

	return client.GetClient(ctx, &api_proto.GetClientRequest{
		ClientId: *shell_client,
	})
}

func doShell() {
	config_obj, err := load_config_or_api()
	kingpin.FatalIfError(err, "Unable to load config file")

	if config_obj.ApiConfig == nil ||
		config_obj.ApiConfig.Name == "" {
		kingpin.Fatalf("Shell requires a valid api config. Generate one with `velociraptor config api_config my_config.yaml --name myName --role administrator`")
	}

	scope := vql_subsystem.MakeScope()
	ctx := InstallSignalHandler(scope)
	client_info, err := getClientInfo(config_obj, ctx)
	kingpin.FatalIfError(err, "Unable to contact server ")

	if client_info.LastIp == "" {
		kingpin.Fatalf("Unknown client %v", *shell_client)
	}

	p := prompt.New(
		func(t string) {
			shell_executor(
				config_obj, ctx, *shell_client, t)
		},
		completer,
		prompt.OptionPrefix(fmt.Sprintf("%v (%v) >",
			*shell_client, client_info.OsInfo.Fqdn)),
	)
	p.Run()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "shell":
			doShell()
		default:
			return false
		}
		return true
	})
}
