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
	"fmt"
	"io/ioutil"
	"regexp"
	"sort"
	"strings"

	"github.com/Velocidex/yaml/v2"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	vql_info        = app.Command("vql", "Show information about the VQL subsystem")
	vql_info_list   = vql_info.Command("list", "Print all VQL plugins and functions")
	vql_info_export = vql_info.Command("export", "Export a YAML file with all the VQL plugins/function descriptions")

	vql_info_export_old_file = vql_info_export.Arg(
		"old_file", "Previous description file will contain additional descriptions").
		File()

	doc_regex = regexp.MustCompile("doc=(.+)")
)

func formatPlugins(
	scope types.Scope,
	info *types.ScopeInformation,
	type_map *types.TypeMap) string {
	records := make(map[string]string)
	names := []string{}

	for _, item := range info.Plugins {
		record := fmt.Sprintf("## %s\n\n%s\n\n", item.Name, item.Doc)
		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			record += "Arg | Description | Type\n"
			record += "----|-------------|-----\n"
			for _, k := range arg_desc.Fields.Keys() {
				v_any, _ := arg_desc.Fields.Get(k)
				v, ok := v_any.(*types.TypeReference)
				if !ok {
					continue
				}

				target := v.Target
				if v.Repeated {
					target = " list of " + target
				}

				required := ""
				if strings.Contains(v.Tag, "required") {
					required = "(required)"
				}
				doc := ""
				matches := doc_regex.FindStringSubmatch(v.Tag)
				if matches != nil {
					doc = matches[1]
				}
				record += fmt.Sprintf(
					"%s | %s | %s %s\n", k, doc, target, required)
			}
		}

		records[item.Name] = record
		names = append(names, item.Name)
	}

	sort.Strings(names)

	result := []string{}
	for _, name := range names {
		result = append(result, records[name])
	}

	return strings.Replace(strings.Join(result, "\n"), "types.Any", "Any", -1)
}

func formatFunctions(
	scope types.Scope,
	info *types.ScopeInformation,
	type_map *types.TypeMap) string {
	records := make(map[string]string)
	names := []string{}

	for _, item := range info.Functions {
		record := fmt.Sprintf("## %s\n\n%s\n\n", item.Name, item.Doc)
		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			record += "Arg | Description | Type\n"
			record += "----|-------------|-----\n"
			for _, k := range arg_desc.Fields.Keys() {
				v_any, _ := arg_desc.Fields.Get(k)
				v, ok := v_any.(*types.TypeReference)
				if !ok {
					continue
				}

				target := v.Target
				if v.Repeated {
					target = " list of " + target
				}

				required := ""
				if strings.Contains(v.Tag, "required") {
					required = "(required)"
				}
				doc := ""
				matches := doc_regex.FindStringSubmatch(v.Tag)
				if matches != nil {
					doc = matches[1]
				}
				record += fmt.Sprintf(
					"%s | %s | %s %s\n", k, doc, target, required)
			}
		}

		records[item.Name] = record
		names = append(names, item.Name)
	}

	sort.Strings(names)

	result := []string{}
	for _, name := range names {
		result = append(result, records[name])
	}

	return strings.Replace(strings.Join(result, "\n"), "types.Any", "Any", -1)
}

func doVQLList() {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	type_map := types.NewTypeMap()
	info := scope.Describe(type_map)

	fmt.Println("VQL Functions")
	fmt.Println("=============")
	fmt.Println("")
	fmt.Println(formatFunctions(scope, info, type_map))

	fmt.Println("VQL Plugins")
	fmt.Println("===========")
	fmt.Println("")
	fmt.Println(formatPlugins(scope, info, type_map))
}

type ArgDesc struct {
	Name        string
	Description string
	Type        string
	Repeated    bool
	Required    bool
}

type PluginDesc struct {
	Name        string
	Description string
	Type        string
	Args        []*ArgDesc
	Category    string
}

func getOldItem(name, item_type string, old_data []*PluginDesc) *PluginDesc {
	for _, item := range old_data {
		if item.Name == name && item.Type == item_type {
			return item
		}
	}
	return nil
}

func doVQLExport() {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	type_map := types.NewTypeMap()
	info := scope.Describe(type_map)

	old_data := []*PluginDesc{}
	if vql_info_export_old_file != nil {
		data, err := ioutil.ReadAll(*vql_info_export_old_file)
		if err == nil {
			err = yaml.Unmarshal(data, &old_data)
			kingpin.FatalIfError(err, "Unmarshal file")
		}
	}

	new_data := []*PluginDesc{}
	seen_plugins := make(map[string]bool)
	seen_functions := make(map[string]bool)

	for _, item := range info.Plugins {
		seen_plugins[item.Name] = true

		new_item := getOldItem(item.Name, "Plugin", old_data)
		if new_item == nil {
			new_item = &PluginDesc{
				Name:        item.Name,
				Description: item.Doc,
				Type:        "Plugin",
			}
		} else {
			// Override the args
			new_item.Args = nil
		}

		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			for _, k := range arg_desc.Fields.Keys() {
				v_any, _ := arg_desc.Fields.Get(k)
				v, ok := v_any.(*types.TypeReference)
				if !ok {
					continue
				}

				arg := &ArgDesc{
					Repeated: v.Repeated,
					Name:     k,
					Type:     v.Target,
				}

				if strings.Contains(v.Tag, "required") {
					arg.Required = true
				}

				matches := doc_regex.FindStringSubmatch(v.Tag)
				if matches != nil {
					arg.Description = matches[1]
				}

				new_item.Args = append(new_item.Args, arg)
			}
		}
		new_data = append(new_data, new_item)
	}

	for _, item := range info.Functions {
		seen_functions[item.Name] = true

		new_item := getOldItem(item.Name, "Function", old_data)
		if new_item == nil {
			new_item = &PluginDesc{
				Name:        item.Name,
				Description: item.Doc,
				Type:        "Function",
			}
		} else {
			// Override the args
			new_item.Args = nil
		}

		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			for _, k := range arg_desc.Fields.Keys() {
				v_any, _ := arg_desc.Fields.Get(k)
				v, ok := v_any.(*types.TypeReference)
				if !ok {
					continue
				}

				arg := &ArgDesc{
					Repeated: v.Repeated,
					Type:     v.Target,
					Name:     k,
				}

				if strings.Contains(v.Tag, "required") {
					arg.Required = true
				}

				matches := doc_regex.FindStringSubmatch(v.Tag)
				if matches != nil {
					arg.Description = matches[1]
				}

				new_item.Args = append(new_item.Args, arg)
			}
		}
		new_data = append(new_data, new_item)
	}

	// Add old data which have not been seen (This can happen if
	// the last export was generated by a different arch than this
	// one so some plugins were not registered).
	for _, item := range old_data {
		if item.Type == "Plugin" {
			_, pres := seen_plugins[item.Name]
			if !pres {
				new_data = append(new_data, item)
			}
		} else if item.Type == "Function" {
			_, pres := seen_functions[item.Name]
			if !pres {
				new_data = append(new_data, item)
			}
		}
	}

	// Sort to maintain stable output.
	sort.Slice(new_data, func(i, j int) bool {
		if new_data[i].Name == new_data[j].Name {
			return new_data[i].Type < new_data[j].Type
		}

		return new_data[i].Name < new_data[j].Name
	})

	serialized, err := yaml.Marshal(new_data)
	kingpin.FatalIfError(err, "Marshal")

	fmt.Println("# Autogenerated! Do not edit.")
	fmt.Println(string(serialized))
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case vql_info_list.FullCommand():
			doVQLList()

		case vql_info_export.FullCommand():
			doVQLExport()

		default:
			return false
		}
		return true
	})
}
