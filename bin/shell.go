package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/c-bata/go-prompt"
	"github.com/google/shlex"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/urns"
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

func shell_executor(config_obj *api_proto.Config,
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
				Name: "Artifact Shell",
				VQL: "SELECT now() as Timestamp, Argv, Stdout, " +
					"Stderr, ReturnCode FROM execve(" +
					"argv=parse_json(data=Argv).Argv)",
			},
		},
	}

	urn := urns.BuildURN(
		"clients", *shell_client,
		"flows", constants.MONITORING_WELL_KNOWN_FLOW)

	err = flows.QueueAndNotifyClient(
		config_obj, *shell_client,
		urn, "VQLClientAction",
		vql_request, processVQLResponses)

	kingpin.FatalIfError(err, "Sending client message ")

	// Wait until the response arrives. The client may not be
	// online so this may block indefinitely!
	// TODO: Implement a timeout for shell commands.
	get_responses(ctx, config_obj, *shell_client)
}

// Create a monitoring event query to receive shell responses and
// write them in the terminal.
func get_responses(ctx context.Context,
	config_obj *api_proto.Config, client_id string) {

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

	env := vfilter.NewDict().
		Set("server_config", config_obj).
		Set("ClientId", client_id)

	scope := vql_subsystem.MakeScope().AppendVars(env)
	defer scope.Close()

	vql, err := vfilter.Parse(`SELECT ReturnCode, Stdout, Stderr, Timestamp,
           Argv FROM watch_monitoring(client_id=ClientId, artifact='Shell')`)
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
		return
	}
}

func completer(t prompt.Document) []prompt.Suggest {
	return []prompt.Suggest{}
}

func getClientInfo(config_obj *api_proto.Config, ctx context.Context) (*api_proto.ApiClient, error) {
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
