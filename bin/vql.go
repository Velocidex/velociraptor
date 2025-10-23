/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"regexp"
	"sort"
	"strings"

	"github.com/Velocidex/yaml/v2"
	"www.velocidex.com/golang/velociraptor/accessors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vutils "www.velocidex.com/golang/velociraptor/utils"
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
		if strings.HasPrefix(item.Doc, "Unimplemented") {
			continue
		}

		record := fmt.Sprintf("## %s\n\n%s\n\n", item.Name, item.Doc)
		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			record += "Arg | Description | Type\n"
			record += "----|-------------|-----\n"
			for _, i := range arg_desc.Fields.Items() {
				v, ok := i.Value.(*types.TypeReference)
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
					"%s | %s | %s %s\n", i.Key, doc, target, required)
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
		if strings.HasPrefix(item.Doc, "Unimplemented") {
			continue
		}

		record := fmt.Sprintf("## %s\n\n%s\n\n", item.Name, item.Doc)
		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			record += "Arg | Description | Type\n"
			record += "----|-------------|-----\n"
			for _, i := range arg_desc.Fields.Items() {
				v, ok := i.Value.(*types.TypeReference)
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
					"%s | %s | %s %s\n", i.Key, doc, target, required)
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
	for _, description := range accessors.DescribeAccessors() {
		fmt.Printf("**%s**: %s\n\n", description.Name,
			description.Description)
	}

	return nil
}

func getOldItem(name, item_type string,
	old_data []*api_proto.Completion) *api_proto.Completion {
	for _, item := range old_data {
		if item.Name == name && item.Type == item_type {
			return item
		}
	}
	return nil
}

func exportAccessors(old_data []*api_proto.Completion) []*api_proto.Completion {
	var completions []*api_proto.Completion

	platform := vql_subsystem.GetMyPlatform()

	lookup := make(map[string]*accessors.AccessorDescriptor)

	for _, description := range accessors.DescribeAccessors() {
		lookup[description.Name] = description
		var metadata map[string]string
		desc_md := description.Metadata()
		if desc_md.Len() > 0 {
			metadata = make(map[string]string)
			for _, i := range desc_md.Items() {
				metadata[i.Key] = utils.ToString(i.Value)
			}
		}

		new_item := getOldItem(description.Name, "Accessor", old_data)
		if new_item == nil {
			new_item = &api_proto.Completion{
				Name:        description.Name,
				Description: description.Description,
				Type:        "Accessor",
				Metadata:    metadata,
			}
		} else {
			// Update the record with new information
			new_item.Metadata = metadata
		}

		if !vutils.InString(new_item.Platforms, platform) {
			new_item.Platforms = append(new_item.Platforms, platform)
			sort.Strings(new_item.Platforms)
		}

		if description.ArgType != nil {
			scope := vql_subsystem.MakeScope()
			type_map := types.NewTypeMap()
			arg_desc, ok := type_map.Get(scope,
				type_map.AddType(scope, description.ArgType))
			if ok {
				addTypeDescription(new_item, arg_desc)
			}
		}

		completions = append(completions, new_item)
	}

	// Also copy all existing accessors which are not known by this
	// implementation. They could be defined in other architectures.
	for _, i := range old_data {
		if i.Type == "Accessor" {
			_, pres := lookup[i.Name]
			if !pres {
				completions = append(completions, i)
			}
		}
	}

	return completions
}

func doVQLExport() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().LoadAndValidate()
	if err != nil {
		config_obj = config.GetDefaultConfig()
	}

	_ = initFilestoreAccessor(config_obj)

	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	type_map := types.NewTypeMap()
	info := scope.Describe(type_map)

	old_data := []*api_proto.Completion{}
	if vql_info_export_old_file != nil {
		data, err := utils.ReadAllWithLimit(*vql_info_export_old_file,
			constants.MAX_MEMORY)
		if err == nil {
			err = yaml.Unmarshal(data, &old_data)
			if err != nil {
				return fmt.Errorf("Unmarshal file: %w", err)
			}
		}
	}

	seen_plugins := make(map[string]bool)
	seen_functions := make(map[string]bool)
	platform := vql_subsystem.GetMyPlatform()
	new_data := exportAccessors(old_data)

	for _, item := range info.Plugins {
		if strings.HasPrefix(item.Doc, "Unimplemented") {
			continue
		}

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
			for _, i := range item.Metadata.Items() {
				metadata[i.Key] = utils.ToString(i.Value)
			}
		}

		if new_item == nil {
			new_item = &api_proto.Completion{
				Name:         item.Name,
				Description:  item.Doc,
				Version:      uint64(item.Version),
				Type:         "Plugin",
				Metadata:     metadata,
				FreeFormArgs: item.FreeFormArgs,
			}
		} else {
			// Override the args and update the version
			new_item.Args = nil
			new_item.Version = uint64(item.Version)
			new_item.Metadata = metadata
			new_item.FreeFormArgs = item.FreeFormArgs
		}

		if !vutils.InString(new_item.Platforms, platform) {
			new_item.Platforms = append(new_item.Platforms, platform)
			sort.Strings(new_item.Platforms)
		}

		new_item.Platforms = utils.DeduplicateStringSlice(new_item.Platforms)
		sort.Strings(new_item.Platforms)

		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			addTypeDescription(new_item, arg_desc)
		}
		new_data = append(new_data, new_item)
	}

	for _, item := range info.Functions {
		if strings.HasPrefix(item.Doc, "Unimplemented") {
			continue
		}

		seen_functions[item.Name] = true

		new_item := getOldItem(item.Name, "Function", old_data)
		var metadata map[string]string
		if item.Metadata != nil {
			metadata = make(map[string]string)
			for _, i := range item.Metadata.Items() {
				metadata[i.Key] = utils.ToString(i.Value)
			}
		}

		if new_item == nil {
			new_item = &api_proto.Completion{
				Name:         item.Name,
				Description:  item.Doc,
				Version:      uint64(item.Version),
				Type:         "Function",
				Metadata:     metadata,
				FreeFormArgs: item.FreeFormArgs,
			}
		} else {
			// Override the args
			new_item.Args = nil
			new_item.Version = uint64(item.Version)
			new_item.Metadata = metadata
			new_item.FreeFormArgs = item.FreeFormArgs
		}

		if !vutils.InString(new_item.Platforms, platform) {
			new_item.Platforms = append(new_item.Platforms, platform)
			sort.Strings(new_item.Platforms)
		}

		new_item.Platforms = utils.DeduplicateStringSlice(new_item.Platforms)
		sort.Strings(new_item.Platforms)

		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			addTypeDescription(new_item, arg_desc)
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

func addTypeDescription(new_item *api_proto.Completion, arg_desc *types.TypeDescription) {
	// Clear the old args
	new_item.Args = nil

	for _, i := range arg_desc.Fields.Items() {
		v, ok := i.Value.(*types.TypeReference)
		if !ok {
			continue
		}

		arg := &api_proto.ArgDescriptor{
			Repeated: v.Repeated,
			Name:     i.Key,
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
