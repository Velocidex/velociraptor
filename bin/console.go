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
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"

	prompt "github.com/c-bata/go-prompt"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/utils"
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
				Default(".velociraptor_history.json").String()
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

	args := strings.Split(t, " ")
	switch strings.ToUpper(args[0]) {
	case "SELECT", "LET":
		executeVQL(config_obj, scope, state, t)

	case "HELP":
		executeHelp(config_obj, scope, state, t)

	case "SET":
		executeSET(config_obj, scope, state, t)

	case "PRINT":
		executePRINT(config_obj, scope, state, t)

	}
}

func executePRINT(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	state *consoleState,
	t string) {

	state.History = append(state.History, t)

	args := strings.Split(t, " ")
	for _, arg := range args {
		value, pres := scope.Resolve(arg)
		if pres {
			utils.Debug(value)
		}
	}
}

func executeSET(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	state *consoleState,
	t string) {

	state.History = append(state.History, t)

	set_re := regexp.MustCompile(`(?i)^\s*SET\s*([^\s]+)\s*=\s*(.+)`)
	matches := set_re.FindStringSubmatch(t)
	if len(matches) > 1 {
		ConsoleLog.Info("Setting %v to %v\n", matches[1], matches[2])
		scope.AppendVars(vfilter.NewDict().Set(matches[1], matches[2]))
	}
}

func executeHelp(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	state *consoleState,
	t string) {

	state.History = append(state.History, t)

	args := strings.Split(t, " ")
	for _, arg := range args {
		if arg == "" || strings.ToUpper(arg) == "HELP" {
			continue
		}

		if strings.HasPrefix(arg, "Artifact.") {
			name := strings.TrimPrefix(arg, "Artifact.")
			repository, err := artifacts.GetGlobalRepository(config_obj)
			if err != nil {
				return
			}

			artifact, pres := repository.Get(name)
			if !pres {
				ConsoleLog.Error("Unknown artifact %s\n", name)
			} else {
				pager, err := GetPager(scope)
				if err != nil {
					ConsoleLog.Error("Cant execute pager: %v\n", err)
					return
				}

				if pager != nil {
					defer pager.Close()

					pager.Writer.Write([]byte(artifact.Raw + "\n"))
					return
				}

				ConsoleLog.Markup(markupArtifact(artifact.Raw))
				return
			}

		} else {
			type_map := vfilter.NewTypeMap()
			descriptions := scope.Describe(type_map)
			for _, function := range descriptions.Functions {
				if function.Name == arg {
					ConsoleLog.Markup(
						fmt.Sprintf(`
Function <important>%v</>:
  <doc>%v</>

`, function.Name, function.Doc))

					arg_type, pres := type_map.Get(scope, function.ArgType)
					if pres {
						renderArgs(arg_type)
					}
				}
			}

			for _, plugin := range descriptions.Plugins {
				if plugin.Name == arg {
					ConsoleLog.Markup(fmt.Sprintf(`
VQL Plugin <important>%v</>:
 <doc>%v</>

`, plugin.Name, plugin.Doc))

					arg_type, pres := type_map.Get(scope, plugin.ArgType)
					if pres {
						renderArgs(arg_type)
					}
				}
			}

		}
	}
}

func renderArgs(type_desc *vfilter.TypeDescription) {
	re := regexp.MustCompile("doc=(.[^,]+)")
	required_re := regexp.MustCompile("(^|,)required(,|$)")

	ConsoleLog.Markup("Args:\n")
	for field, desc := range type_desc.Fields {
		repeated := ""
		if desc.Repeated {
			repeated = "<repeated>repeated</>"
		}

		required := ""
		if required_re.FindString(desc.Tag) != "" {
			required = "<required>required</>"
		}

		doc := ""
		matches := re.FindStringSubmatch(desc.Tag)
		if len(matches) > 0 {
			doc = matches[1]
		}

		ConsoleLog.Markup(
			fmt.Sprintf("  <name>%v</>: %v (<type>%v</>) %v %v\n",
				field, doc, desc.Target, repeated, required))
	}
	ConsoleLog.Markup("\n")
}

func executeVQL(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	state *consoleState,
	t string) {
	vql, err := vfilter.Parse(t)
	if err != nil {
		ConsoleLog.Error("Error: %v\n", err)
		return
	}

	state.History = append(state.History, t)

	ctx, cancel := install_sig_handler()
	defer cancel()

	format := *console_format
	env_format, pres := scope.Resolve("FORMAT")
	if pres {
		format, _ = env_format.(string)
	}

	var out io.WriteCloser = os.Stdout
	pager, err := GetPager(scope)
	if err != nil {
		ConsoleLog.Error("Cant execute pager: %v\n", err)
		return
	}

	if pager != nil {
		out = pager.Writer
		defer pager.Close()
	}

	switch format {
	case "text":
		table := evalQueryToTable(ctx, scope, vql, out)
		table.Render()
	case "json":
		outputJSON(ctx, scope, vql, out)
	case "csv":
		outputCSV(ctx, scope, vql, out)
	default:
		ConsoleLog.Error("Unknown format, to fix issue - SET FORMAT = json\n")
	}
}

var toplevel_commands = []prompt.Suggest{
	{Text: "SELECT", Description: "Start a query"},
	{Text: "LET", Description: "Assign a stored query"},
	{Text: "HELP", Description: "Show help about plugins, functions etc"},
	{Text: "SET", Description: "Set an environment variable"},
	{Text: "PRINT", Description: "Print an environment variable"},
}

func console_completer(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	d prompt.Document) []prompt.Suggest {
	if d.TextBeforeCursor() == "" {
		return []prompt.Suggest{}
	}

	args := strings.Split(d.TextBeforeCursor(), " ")
	if len(args) <= 1 {
		return prompt.FilterHasPrefix(toplevel_commands, args[0], true)
	}

	current_word := d.GetWordBeforeCursor()

	switch strings.ToUpper(args[0]) {
	case "SELECT":
		return completeSELECT(config_obj, scope, args, current_word)

	case "LET":
		return completeLET(config_obj, scope, args, current_word)

	case "HELP":
		return completeHELP(config_obj, scope, args, current_word)

	case "SET":
		return completeSET(config_obj, scope, args, current_word)

	case "PRINT":
		return completePRINT(config_obj, scope, args, current_word)
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

func suggestPlugins(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	add_bracket bool) []prompt.Suggest {
	result := []prompt.Suggest{}

	type_map := vfilter.NewTypeMap()
	descriptions := scope.Describe(type_map)
	for _, plugin := range descriptions.Plugins {
		name := plugin.Name
		if add_bracket {
			name += "("
		}

		result = append(result, prompt.Suggest{
			Text: name, Description: plugin.Doc},
		)
	}

	// Now fill in the artifacts.
	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return result
	}
	for _, name := range repository.List() {
		artifact, pres := repository.Get(name)
		if pres {
			if add_bracket {
				name += "("
			}

			result = append(result, prompt.Suggest{
				Text:        "Artifact." + name,
				Description: artifact.Description,
			})
		}
	}
	return result
}

func suggestFunctions(scope *vfilter.Scope,
	add_bracket bool) []prompt.Suggest {
	result := []prompt.Suggest{}

	type_map := vfilter.NewTypeMap()
	descriptions := scope.Describe(type_map)
	for _, function := range descriptions.Functions {
		name := function.Name
		if add_bracket {
			name += "("
		}
		result = append(result, prompt.Suggest{
			Text: name, Description: function.Doc},
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

func completePRINT(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	args []string,
	current_word string) []prompt.Suggest {

	columns := suggestVars(scope)

	sort.Slice(columns, func(i, j int) bool {
		return columns[i].Text < columns[j].Text
	})

	return prompt.FilterHasPrefix(columns, current_word, true)
}

func completeSET(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	args []string,
	current_word string) []prompt.Suggest {

	columns := []prompt.Suggest{
		{Text: "FORMAT", Description: "Control the output format."},
		{Text: "PAGER", Description: "Specify the output pager."},
	}

	sort.Slice(columns, func(i, j int) bool {
		return columns[i].Text < columns[j].Text
	})

	return prompt.FilterHasPrefix(columns, current_word, true)
}

func completeHELP(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	args []string,
	current_word string) []prompt.Suggest {
	columns := append(suggestFunctions(scope, false),
		suggestPlugins(config_obj, scope, false)...)

	sort.Slice(columns, func(i, j int) bool {
		return columns[i].Text < columns[j].Text
	})

	return prompt.FilterHasPrefix(columns, current_word, true)
}

func completeLET(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	args []string,
	current_word string) []prompt.Suggest {
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
		return completeSELECT(config_obj,
			scope, args[3:], current_word)
	}

	sort.Slice(columns, func(i, j int) bool {
		return columns[i].Text < columns[j].Text
	})

	return prompt.FilterHasPrefix(columns, current_word, true)
}

func completeSELECT(
	config_obj *api_proto.Config,
	scope *vfilter.Scope,
	args []string,
	current_word string) []prompt.Suggest {
	last_word := ""
	previous_word := ""
	for _, w := range args {
		if w != "" {
			previous_word = last_word
			last_word = w
		}
	}

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
			columns = append(columns, suggestFunctions(scope, true)...)

			// * is only valid immediately after SELECT
		} else if strings.HasSuffix(last_word, ",") || current_word != "" {
			columns = append(columns, suggestVars(scope)...)
			columns = append(columns, suggestFunctions(scope, true)...)
		}

	} else {
		if strings.ToUpper(last_word) == "FROM" ||
			current_word != "" &&
				strings.ToUpper(previous_word) == "FROM" {
			columns = append(columns, suggestVars(scope)...)
			columns = append(columns, suggestPlugins(config_obj, scope, true)...)

		} else if !NoCaseInString(args, "WHERE") {
			columns = append(columns, prompt.Suggest{
				Text: "WHERE", Description: "Condition to filter the result set"},
			)
			columns = append(columns, suggestLimit(scope)...)

		} else {
			columns = append(columns, suggestLimit(scope)...)
			columns = append(columns, suggestVars(scope)...)
			columns = append(columns, suggestFunctions(scope, true)...)
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
			return console_completer(config_obj, scope, d)
		},
		prompt.OptionPrefix("VQL > "),
		prompt.OptionHistory(state.History),
		prompt.OptionMaxSuggestion(10),
		prompt.OptionAddKeyBind(prompt.KeyBind{
			Key: prompt.ControlDelete,
			Fn:  prompt.DeleteWord,
		}),
	)
	p.Run()
}

var yaml_keywords = regexp.MustCompile(
	`(name:|description:|parameters:|sources:|precondition:|queries:)`)

var VQL_keywords = regexp.MustCompile(
	`(SELECT|LET|FROM)`)

func markupArtifact(str string) string {
	str = yaml_keywords.ReplaceAllString(str, `<keyword>$1</>`)
	return VQL_keywords.ReplaceAllString(str, `<VQL>$1</>`)
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
