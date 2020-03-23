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
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	prompt "github.com/c-bata/go-prompt"
	"github.com/google/shlex"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config "www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	// Command line interface for VQL commands.
	shell        = app.Command("shell", "Run an interactive shell on a client")
	shell_client = shell.Arg("client_id", "The client id to run the shell for.").
			Required().String()
)

const (
	_                          = iota
	processVQLResponses uint64 = iota
)

func shell_executor(config_obj *config_proto.Config,
	ctx context.Context,
	t string) {

	if t == "" {
		return
	}

	argv, err := shlex.Split(t)
	if err != nil {
		// Not a fatal error - the user can retry.
		fmt.Printf("Parsing command line: %v\n", err)
		return
	}

	fmt.Printf("Running %v on %v\n", t, *shell_client)

	// Query environment is a string and we need to send an array
	// - we json marshal it and unpack it in VQL.
	encoded_argv, err := json.Marshal(&map[string]interface{}{"Argv": argv})
	kingpin.FatalIfError(err, "Argv ")

	vql_request := &actions_proto.VQLCollectorArgs{
		Env: []*actions_proto.VQLEnv{
			&actions_proto.VQLEnv{
				Key:   "Argv",
				Value: string(encoded_argv),
			},
		},
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				Name: "Server.Internal.Shell",
				VQL: "SELECT now() as Timestamp, Argv, Stdout, " +
					"Stderr, ReturnCode FROM execve(" +
					"argv=parse_json(data=Argv).Argv)",
			},
		},
	}

	wg := &sync.WaitGroup{}

	// There is a race condition here - it takes a short time for
	// watch_monitoring to get set up and the client may return
	// results before then.
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Wait until the response arrives. The client may not be
		// online so this may block indefinitely!
		// TODO: Implement a timeout for shell commands.
		get_responses(ctx, config_obj, *shell_client)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Obfuscate the artifact from the client. It will be
		// automatically deobfuscated when the client replies to the
		// monitoring flow.
		artifacts.Obfuscate(config_obj, vql_request)

		err = flows.QueueMessageForClient(
			config_obj, *shell_client,
			&crypto_proto.GrrMessage{
				SessionId:       constants.MONITORING_WELL_KNOWN_FLOW,
				RequestId:       processVQLResponses,
				VQLClientAction: vql_request})
		kingpin.FatalIfError(err, "Sending client message ")

		api_client_factory := grpc_client.GRPCAPIClient{}
		client, cancel := api_client_factory.GetAPIClient(config_obj)
		defer cancel()

		_, err = client.NotifyClients(context.Background(),
			&api_proto.NotificationRequest{ClientId: *shell_client})
		kingpin.FatalIfError(err, "Sending client message ")
	}()

	wg.Wait()
}

// Create a monitoring event query to receive shell responses and
// write them in the terminal.
func get_responses(ctx context.Context,
	config_obj *config_proto.Config, client_id string) {

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		for {
			select {
			case <-sub_ctx.Done():
				return

			case <-c:
				fmt.Println("Cancelled!")
				cancel()
			}
		}
	}()

	env := ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR,
			vql_subsystem.NewRoleACLManager("administrator")).
		Set("server_config", config_obj).
		Set("ClientId", client_id)

	scope := vql_subsystem.MakeScope().AppendVars(env)
	defer scope.Close()

	vql, err := vfilter.Parse(`SELECT ReturnCode, Stdout, Stderr, Timestamp,
           Argv FROM watch_monitoring(client_id=ClientId, artifact='Server.Internal.Shell')`)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to parse VQL Query")
	}

	print_value := func(row vfilter.Row, field string) {
		value, ok := scope.Associative(row, field)
		if ok {
			fmt.Printf("%v\n", value)
		}
	}

	for row := range vql.Eval(sub_ctx, scope) {
		return_code, _ := scope.Associative(row, "ReturnCode")
		ts_any, ok := scope.Associative(row, "Timestamp")
		if ok {
			ts_str, ok := ts_any.(string)
			if ok {
				ts, err := strconv.Atoi(ts_str)
				if err == nil {
					fmt.Printf("Received response at %v - "+
						"Return code %v\n",
						time.Unix(int64(ts), 0), return_code)
				}
			}
		}

		print_value(row, "Stderr")
		print_value(row, "Stdout")

		break
	}
}

func completer(t prompt.Document) []prompt.Suggest {
	return []prompt.Suggest{}
}

func getClientInfo(config_obj *config_proto.Config, ctx context.Context) (*api_proto.ApiClient, error) {
	channel := grpc_client.GetChannel(config_obj)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	return client.GetClient(context.Background(), &api_proto.GetClientRequest{
		ClientId: *shell_client,
	})
}

func doShell() {
	config_obj, err := config.LoadConfig(*config_path)
	if err != nil {
		config_obj = config.GetDefaultConfig()
	}

	// Preload the repository before we start querying it. Loading
	// the repository later will result in a race condition.
	_, err = artifacts.GetGlobalRepository(config_obj)
	kingpin.FatalIfError(err, "Unable load global repository")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client_info, err := getClientInfo(config_obj, ctx)
	kingpin.FatalIfError(err, "Unable to contact server ")

	if client_info.LastIp == "" {
		kingpin.Fatalf("Unknown client %v", *shell_client)
	}

	p := prompt.New(
		func(t string) {
			shell_executor(config_obj, ctx, t)
		},
		completer,
		prompt.OptionPrefix(fmt.Sprintf("%v (%v) >", *shell_client, client_info.OsInfo.Fqdn)),
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
