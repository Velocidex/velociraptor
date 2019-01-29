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
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"

	prompt "github.com/c-bata/go-prompt"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vql_networking "www.velocidex.com/golang/velociraptor/vql/networking"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	// Command line interface for VQL commands.
	console        = app.Command("console", "Enter the interactive console")
	console_format = console.Flag("format", "Output format to use.").
			Default("json").Enum("text", "json", "csv")
	console_dump_dir = console.Flag("dump_dir", "Directory to dump output files.").
				Default(".").String()

	console_history_file = console.Flag("history", "Filename to store history in.").
				Default("/tmp/velociraptor_history").String()
)

type consoleState struct {
	History []string
}

func console_executor(config_obj *api_proto.Config,
	scope *vfilter.Scope,
	state *consoleState,
	t string) {

	if t == "" {
		return
	}

	vql, err := vfilter.Parse(t)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	state.History = append(state.History, t)

	ctx, cancel := install_sig_handler()
	defer cancel()

	switch *console_format {
	case "text":
		table := evalQueryToTable(ctx, scope, vql)
		table.Render()
	case "json":
		outputJSON(ctx, scope, vql)
	case "csv":
		outputCSV(ctx, scope, vql)
	}
}

var toplevel_commands = []prompt.Suggest{
	{Text: "SELECT", Description: "Start a query"},
	{Text: "LET", Description: "Assign a stored query"},
}

func console_completer(scope *vfilter.Scope, d prompt.Document) []prompt.Suggest {
	if d.TextBeforeCursor() == "" {
		return []prompt.Suggest{}
	}

	args := strings.Split(d.TextBeforeCursor(), " ")
	if len(args) <= 1 {
		return prompt.FilterHasPrefix(toplevel_commands, args[0], true)
	}

	if strings.ToUpper(args[0]) == "SELECT" {
		return completeSELECT(scope, args)
	}

	if strings.ToUpper(args[0]) == "LET" {
		return completeLET(scope, args)
	}

	return []prompt.Suggest{}
}

func NoCaseInString(hay []string, needle string) bool {
	needle = strings.ToUpper(needle)

	for _, x := range hay {
		if strings.ToUpper(x) == needle {
			return true
		}
	}

	return false
}

func suggestVars(scope *vfilter.Scope) []prompt.Suggest {
	result := []prompt.Suggest{}
	for _, member := range scope.Keys() {
		// Skip hidden internal vars
		if strings.HasPrefix(member, "$") {
			continue
		}
		if strings.HasPrefix(member, "_") {
			continue
		}

		result = append(result, prompt.Suggest{
			Text: member,
		})
	}
	return result
}

func suggestPlugins(scope *vfilter.Scope) []prompt.Suggest {
	result := []prompt.Suggest{}

	type_map := vfilter.NewTypeMap()
	descriptions := scope.Describe(type_map)
	for _, plugin := range descriptions.Plugins {
		result = append(result, prompt.Suggest{
			Text: plugin.Name + "(", Description: plugin.Doc},
		)
	}
	return result
}

func suggestFunctions(scope *vfilter.Scope) []prompt.Suggest {
	result := []prompt.Suggest{}

	type_map := vfilter.NewTypeMap()
	descriptions := scope.Describe(type_map)
	for _, function := range descriptions.Functions {
		result = append(result, prompt.Suggest{
			Text: function.Name + "(", Description: function.Doc},
		)
	}
	return result
}

func suggestLimit(scope *vfilter.Scope) []prompt.Suggest {
	return []prompt.Suggest{
		{Text: "LIMIT", Description: "Limit to this many rows"},
		{Text: "ORDER BY", Description: "order results by a column"},
	}
}

func completeLET(scope *vfilter.Scope, args []string) []prompt.Suggest {
	columns := []prompt.Suggest{}

	if len(args) == 3 {
		columns = []prompt.Suggest{
			{Text: "=", Description: "Store query in scope"},
			{Text: "<=", Description: "Materialize query in scope"},
		}
	} else if len(args) == 4 {
		columns = []prompt.Suggest{
			{Text: "SELECT", Description: "Start Query"},
		}
	} else if len(args) > 4 && strings.ToUpper(args[3]) == "SELECT" {
		return completeSELECT(scope, args[3:len(args)])
	}

	sort.Slice(columns, func(i, j int) bool {
		return columns[i].Text < columns[j].Text
	})

	return prompt.FilterHasPrefix(columns, args[len(args)-1], true)
}

func completeSELECT(scope *vfilter.Scope, args []string) []prompt.Suggest {
	last_word := ""
	previous_word := ""
	for _, w := range args {
		if w != "" {
			previous_word = last_word
			last_word = w
		}
	}

	current_word := args[len(args)-1]

	columns := []prompt.Suggest{}

	// Sentence does not have a FROM yet complete columns.
	if !NoCaseInString(args, "FROM") {
		columns = append(columns, prompt.Suggest{
			Text: "FROM", Description: "Select from plugin"},
		)

		if strings.ToUpper(last_word) == "SELECT" {
			columns = append(columns, prompt.Suggest{
				Text: "*", Description: "All columns",
			})
			columns = append(columns, suggestVars(scope)...)
			columns = append(columns, suggestFunctions(scope)...)

			// * is only valid immediately after SELECT
		} else if strings.HasSuffix(last_word, ",") || current_word != "" {
			columns = append(columns, suggestVars(scope)...)
			columns = append(columns, suggestFunctions(scope)...)
		}

	} else {
		if strings.ToUpper(last_word) == "FROM" ||
			current_word != "" &&
				strings.ToUpper(previous_word) == "FROM" {
			columns = append(columns, suggestVars(scope)...)
			columns = append(columns, suggestPlugins(scope)...)

		} else if !NoCaseInString(args, "WHERE") {
			columns = append(columns, prompt.Suggest{
				Text: "WHERE", Description: "Condition to filter the result set"},
			)
			columns = append(columns, suggestLimit(scope)...)

		} else {
			columns = append(columns, suggestLimit(scope)...)
			columns = append(columns, suggestVars(scope)...)
			columns = append(columns, suggestFunctions(scope)...)
		}
	}

	sort.Slice(columns, func(i, j int) bool {
		return columns[i].Text < columns[j].Text
	})

	return prompt.FilterHasPrefix(columns, current_word, true)
}

func load_state() *consoleState {
	result := &consoleState{}
	fd, err := os.Open(*console_history_file)
	if err != nil {
		return result
	}

	data, _ := ioutil.ReadAll(fd)
	json.Unmarshal(data, &result)
	return result
}

func save_state(state *consoleState) {
	fd, err := os.OpenFile(*console_history_file, os.O_WRONLY|os.O_CREATE,
		0600)
	if err != nil {
		return
	}

	serialized, err := json.Marshal(state)
	if err != nil {
		return
	}

	fd.Write(serialized)
}

func install_sig_handler() (context.Context, context.CancelFunc) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-quit:
			cancel()

		case <-ctx.Done():
			return
		}
	}()

	return ctx, cancel

}

func doConsole() {
	config_obj := get_config_or_default()
	repository, err := artifacts.GetGlobalRepository(config_obj)
	kingpin.FatalIfError(err, "Artifact GetGlobalRepository ")
	repository.LoadDirectory(*artifact_definitions_dir)

	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("server_config", config_obj).
		Set("$uploader", &vql_networking.FileBasedUploader{
			UploadDir: *console_dump_dir,
		}).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	if env_map != nil {
		for k, v := range *env_map {
			env.Set(k, v)
		}
	}

	scope := artifacts.MakeScope(repository).AppendVars(env)
	defer scope.Close()

	scope.Logger = log.New(os.Stderr, "velociraptor: ", log.Lshortfile)

	state := load_state()
	defer save_state(state)

	p := prompt.New(
		func(t string) {
			console_executor(config_obj, scope, state, t)
		},
		func(d prompt.Document) []prompt.Suggest {
			return console_completer(scope, d)
		},
		prompt.OptionPrefix("-> "),
		prompt.OptionHistory(state.History),
	)
	p.Run()
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case console.FullCommand():
			doConsole()

		default:
			return false
		}
		return true
	})
}
