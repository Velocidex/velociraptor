package main

import (
	"gopkg.in/alecthomas/kingpin.v2"
	"os"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/inspect"
)

var (
	app         = kingpin.New("velociraptor", "An advanced incident response agent.")
	config_path = app.Flag("config", "The configuration file.").String()
	// Command line interface for VQL commands.
	query   = app.Command("query", "Run a VQL query")
	queries = query.Arg("queries", "The VQL Query to run.").
		Required().Strings()
	format = query.Flag("format", "Output format to use.").
		Default("json").Enum("text", "json")
	dump_dir = query.Flag("dump_dir", "Directory to dump output files.").
			Default(".").String()

	explain        = app.Command("explain", "Explain the output from a plugin")
	explain_plugin = explain.Arg("plugin", "Plugin to explain").Required().String()

	// Run the client.
	client = app.Command("client", "Run the velociraptor client")

	// Run the server.
	frontend = app.Command("frontend", "Run the frontend and GUI.")

	// Inspect the filestore
	inspect_command = app.Command(
		"inspect", "Inspect datastore files.")
	inspect_filename = inspect_command.Arg(
		"filename", "The filename from the filestore").
		Required().String()

	config_command = app.Command(
		"config", "Manipulate the configuration.")
	config_show_command = config_command.Command(
		"show", "Show the current config.")
	config_client_command = config_command.Command(
		"client", "Dump the client's config file.")
	config_generate_command = config_command.Command(
		"generate",
		"Generate a new config file to stdout (with new keys).")
	config_rotate_server_key = config_command.Command(
		"rotate_key",
		"Generate a new config file with a rotates server key.")
)

func main() {
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case "config show":
		doShowConfig()

	case "config generate":
		doGenerateConfig()

	case "config rotate_key":
		doRotateKeyConfig()

	case "config client":
		doDumpClientConfig()

	case client.FullCommand():
		RunClient(config_path)

	case explain.FullCommand():
		doExplain(*explain_plugin)

	case query.FullCommand():
		doQuery()

	case repack.FullCommand():
		err := RepackClient(*repack_binary, *repack_config)
		if err != nil {
			kingpin.FatalIfError(err, "Can not repack client")
		}

	case frontend.FullCommand():
		config_obj, err := get_server_config(*config_path)
		kingpin.FatalIfError(err, "Unable to load config file")
		go func() {
			err := api.StartServer(config_obj)
			kingpin.FatalIfError(err, "Unable to start API server")
		}()
		go func() {
			err := api.StartHTTPProxy(config_obj)
			kingpin.FatalIfError(err, "Unable to start HTTP Proxy server")
		}()

		start_frontend(config_obj)

	case inspect_command.FullCommand():
		config_obj, err := get_server_config(*config_path)
		kingpin.FatalIfError(err, "Unable to load config file")
		err = inspect.Inspect(config_obj, *inspect_filename)
		kingpin.FatalIfError(err, "Unable to parse datastore item.")
	}
}
