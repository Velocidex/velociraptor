package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/alecthomas/kingpin.v2"
	"os"
	"www.velocidex.com/golang/velociraptor"
	"www.velocidex.com/golang/vfilter"
)

var (
	query   = kingpin.Command("query", "Run a VQL query")
	queries = query.Arg("query", "The VQL Query to run.").Required().Strings()
	format  = query.Flag("format", "Output format to use.").Default("json").Enum("text", "json")

	explain        = kingpin.Command("explain", "Explain the output from a plugun")
	explain_plugin = explain.Arg("plugun", "Plugin to explain").Required().String()
)

func outputJSON(vql *vfilter.VQL) {
	scope := velociraptor.MakeScope()
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

func evalQuery(vql *vfilter.VQL) {
	scope := velociraptor.MakeScope()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output_chan := vql.Eval(ctx, scope)
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(*vql.Columns(scope))
	defer table.Render()

	columns := vql.Columns(scope)
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
				default:
					cell = fmt.Sprintf("%v", value)
				}
			}
			string_row = append(string_row, cell)
		}

		table.Append(string_row)
	}
}

func doExplain(plugin string) {
	type_map := make(vfilter.TypeMap)
	scope := velociraptor.MakeScope()
	if pslist_info, pres := scope.Info(&type_map, plugin); pres {
		vfilter.Debug(pslist_info)
		vfilter.Debug(type_map)
	}
}

func main() {
	switch kingpin.Parse() {
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
