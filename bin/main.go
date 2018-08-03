package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/alecthomas/kingpin.v2"
	"log"
	"os"
	"strings"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	// Command line interface for VQL commands.
	query   = kingpin.Command("query", "Run a VQL query")
	queries = query.Arg("query", "The VQL Query to run.").
		Required().Strings()
	format = query.Flag("format", "Output format to use.").
		Default("json").Enum("text", "json")
	dump_dir = query.Flag("dump_dir", "Directory to dump output files.").
			Default(".").String()

	explain        = kingpin.Command("explain", "Explain the output from a plugin")
	explain_plugin = explain.Arg("plugin", "Plugin to explain").Required().String()

	// Run the client.
	client             = kingpin.Command("client", "Run the velociraptor client")
	client_config_path = client.Arg("config", "The client's config file.").String()
	show_config        = client.Flag("show_config", "Display the client's configuration").Bool()

	// Run the server.
	frontend       = kingpin.Command("frontend", "Run the frontend.")
	fe_config_path = frontend.Arg("config", "The Configuration file").String()
)

func outputJSON(scope *vfilter.Scope, vql *vfilter.VQL) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result_chan := vfilter.GetResponseChannel(vql, ctx, scope, 10)
	for {
		result, ok := <-result_chan
		if !ok {
			return
		}
		os.Stdout.Write(result.Payload)
	}
}

func hard_wrap(text string, colBreak int) string {
	text = strings.TrimSpace(text)
	wrapped := ""
	var i int
	for i = 0; len(text[i:]) > colBreak; i += colBreak {

		wrapped += text[i:i+colBreak] + "\n"

	}
	wrapped += text[i:]

	return wrapped
}

func evalQuery(scope *vfilter.Scope, vql *vfilter.VQL) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output_chan := vql.Eval(ctx, scope)
	table := tablewriter.NewWriter(os.Stdout)
	defer table.Render()

	columns := vql.Columns(scope)
	table.SetHeader(*columns)
	table.SetCaption(true, vql.ToString(scope))

	for {
		row, ok := <-output_chan
		if !ok {
			return
		}
		string_row := []string{}
		if len(*columns) == 0 {
			members := scope.GetMembers(row)
			table.SetHeader(members)
			columns = &members
		}

		for _, key := range *columns {
			cell := ""
			value, pres := scope.Associative(row, key)
			if pres && !utils.IsNil(value) {
				switch t := value.(type) {
				case vfilter.StringProtocol:
					cell = t.ToString(scope)
				case fmt.Stringer:
					cell = hard_wrap(t.String(), 30)
				case []byte:
					cell = hard_wrap(string(t), 30)
				case string:
					cell = hard_wrap(t, 30)
				default:
					if k, err := json.Marshal(value); err == nil {
						cell = hard_wrap(string(k), 30)
					}
				}
			}
			string_row = append(string_row, cell)
		}

		table.Append(string_row)
	}
}

func doExplain(plugin string) {
	result := vfilter.NewDict()
	type_map := make(vfilter.TypeMap)
	scope := vql_subsystem.MakeScope()
	if pslist_info, pres := scope.Info(&type_map, plugin); pres {
		result.Set(plugin+"_info", pslist_info)
		result.Set("type_map", type_map)
	}

	s, err := json.MarshalIndent(result, "", " ")
	if err == nil {
		os.Stdout.Write(s)
	}
}

func main() {
	switch kingpin.Parse() {
	case "client":
		RunClient(client_config_path)

	case "explain":
		doExplain(*explain_plugin)

	case "query":
		env := vfilter.NewDict().
			Set("$uploader", &vql_subsystem.FileBasedUploader{*dump_dir})
		scope := vql_subsystem.MakeScope().AppendVars(env)

		scope.Logger = log.New(os.Stderr, "vraptor: ", log.Lshortfile)
		for _, query := range *queries {
			vql, err := vfilter.Parse(query)
			if err != nil {
				kingpin.FatalIfError(err, "Unable to parse VQL Query")
			}

			switch *format {
			case "text":
				evalQuery(scope, vql)
			case "json":
				outputJSON(scope, vql)
			}
		}

	case "repack":
		err := RepackClient(*repack_binary, *repack_config)
		if err != nil {
			kingpin.FatalIfError(err, "Can not repack client")
		}

	case "frontend":
		config_obj, err := get_config(*fe_config_path)
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
	}
}
