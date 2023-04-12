/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	"www.velocidex.com/golang/velociraptor/accessors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
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

	doc_regex  = regexp.MustCompile("doc=(.+)")
	type_regex = regexp.MustCompile("type: types.(Any|StoredQuery|LazyExpr)")
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

func doVQLList() error {
	logging.DisableLogging()

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

	fmt.Println("Accessors")
	fmt.Println("===========")
	fmt.Println("")
	description := accessors.DescribeAccessors()
	keys := description.Keys()
	sort.Strings(keys)
	if description != nil {
		for _, k := range keys {
			v, _ := description.Get(k)
			fmt.Printf("%s: %s\n", k, v)
		}
	}

	return nil
}

func getOldItem(name, item_type string, old_data []*api_proto.Completion) *api_proto.Completion {
	for _, item := range old_data {
		if item.Name == name && item.Type == item_type {
			return item
		}
	}
	return nil
}

func doVQLExport() error {
	logging.DisableLogging()

	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	type_map := types.NewTypeMap()
	info := scope.Describe(type_map)

	old_data := []*api_proto.Completion{}
	if vql_info_export_old_file != nil {
		data, err := ioutil.ReadAll(*vql_info_export_old_file)
		if err == nil {
			err = yaml.Unmarshal(data, &old_data)
			if err != nil {
				return fmt.Errorf("Unmarshal file: %w", err)
			}
		}
	}

	new_data := []*api_proto.Completion{}
	seen_plugins := make(map[string]bool)
	seen_functions := make(map[string]bool)

	for _, item := range info.Plugins {
		seen_plugins[item.Name] = true

		// We maintain the following fields from old plugins:
		// - Description
		// - Category
		// And update these fields from the current plugins
		// - Args
		// - Version
		//
		// This means that it is possible to edit the old vql.yaml
		// file to include more detailed description and it wil not be
		// over-ridden by the new plugins. But any new arg
		// descriptions are always copied from the running code.
		new_item := getOldItem(item.Name, "Plugin", old_data)
		var metadata map[string]string
		if item.Metadata != nil {
			metadata = make(map[string]string)
			for _, k := range item.Metadata.Keys() {
				v, _ := item.Metadata.GetString(k)
				metadata[k] = v
			}
		}

		if new_item == nil {
			new_item = &api_proto.Completion{
				Name:        item.Name,
				Description: item.Doc,
				Version:     uint64(item.Version),
				Type:        "Plugin",
				Metadata:    metadata,
			}
		} else {
			// Override the args and update the version
			new_item.Args = nil
			new_item.Version = uint64(item.Version)
			new_item.Metadata = metadata
		}

		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			for _, k := range arg_desc.Fields.Keys() {
				v_any, _ := arg_desc.Fields.Get(k)
				v, ok := v_any.(*types.TypeReference)
				if !ok {
					continue
				}

				arg := &api_proto.ArgDescriptor{
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
		var metadata map[string]string
		if item.Metadata != nil {
			metadata = make(map[string]string)
			for _, k := range item.Metadata.Keys() {
				v, _ := item.Metadata.GetString(k)
				metadata[k] = v
			}
		}

		if new_item == nil {
			new_item = &api_proto.Completion{
				Name:        item.Name,
				Description: item.Doc,
				Version:     uint64(item.Version),
				Type:        "Function",
				Metadata:    metadata,
			}
		} else {
			// Override the args
			new_item.Args = nil
			new_item.Version = uint64(item.Version)
			new_item.Metadata = metadata
		}

		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			for _, k := range arg_desc.Fields.Keys() {
				v_any, _ := arg_desc.Fields.Get(k)
				v, ok := v_any.(*types.TypeReference)
				if !ok {
					continue
				}

				arg := &api_proto.ArgDescriptor{
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
	if err != nil {
		return err
	}

	fmt.Println("# Autogenerated! It is safe to edit descriptions.")
	fmt.Println(type_regex.ReplaceAllString(string(serialized), "type: $1"))
	return nil
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case vql_info_list.FullCommand():
			FatalIfError(vql_info_list, doVQLList)

		case vql_info_export.FullCommand():
			FatalIfError(vql_info_export, doVQLExport)

		default:
			return false
		}
		return true
	})
}
