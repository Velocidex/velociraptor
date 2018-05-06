package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/alecthomas/kingpin.v2"
	"os"
	"strings"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	//	utils 	"www.velocidex.com/golang/velociraptor/testing"
)

var (
	query   = kingpin.Command("query", "Run a VQL query")
	queries = query.Arg("query", "The VQL Query to run.").Required().Strings()
	format  = query.Flag("format", "Output format to use.").Default("json").Enum("text", "json")

	explain        = kingpin.Command("explain", "Explain the output from a plugin")
	explain_plugin = explain.Arg("plugin", "Plugin to explain").Required().String()

	client      = kingpin.Command("client", "Run the velociraptor client")
	config_path = client.Arg("config", "The client's config file.").Required().String()
)

func outputJSON(vql *vfilter.VQL) {
	scope := vql_subsystem.MakeScope()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output_chan := vql.Eval(ctx, scope)
	result := []vfilter.Row{}
	for row := range output_chan {
		result = append(result, row)
	}

	s, err := json.MarshalIndent(result, "", " ")
	if err == nil {
		os.Stdout.Write(s)
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

func evalQuery(vql *vfilter.VQL) {
	scope := vql_subsystem.MakeScope()
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
		for _, key := range *columns {
			cell := ""
			if value, pres := scope.Associative(row, key); pres {
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
	type_map := make(vfilter.TypeMap)
	scope := vql_subsystem.MakeScope()
	if pslist_info, pres := scope.Info(&type_map, plugin); pres {
		vfilter.Debug(pslist_info)
		vfilter.Debug(type_map)
	}
}

func main() {
	switch kingpin.Parse() {
	case "client":
		RunClient()

	case "explain":
		doExplain(*explain_plugin)
	case "query":
		for _, query := range *queries {
			vql, err := vfilter.Parse(query)
			if err != nil {
				kingpin.FatalIfError(err, "Unable to parse VQL Query")
			}

			switch *format {
			case "text":
				evalQuery(vql)
			case "json":
				outputJSON(vql)
			}
		}
	}
}
