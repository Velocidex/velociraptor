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
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
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

func doQuery() {
	config_obj, _ := get_config(*config_path)
	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("$uploader", &vql_subsystem.FileBasedUploader{*dump_dir})
	scope := vql_subsystem.MakeScope().AppendVars(env)

	scope.Logger = log.New(os.Stderr, "velociraptor: ", log.Lshortfile)
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
