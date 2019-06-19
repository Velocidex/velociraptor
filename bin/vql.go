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
	"regexp"
	"sort"
	"strings"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	vql_info      = app.Command("vql", "Show information about the VQL subsystem")
	vql_info_list = vql_info.Command("list", "Print all VQL plugins and functions")

	doc_regex = regexp.MustCompile("doc=(.+)")
)

func formatPlugins(
	scope *vfilter.Scope,
	info *vfilter.ScopeInformation,
	type_map *vfilter.TypeMap) string {
	records := make(map[string]string)
	names := []string{}

	for _, item := range info.Plugins {
		record := fmt.Sprintf("## %s\n\n%s\n\n", item.Name, item.Doc)
		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			record += "Arg | Description | Type\n"
			record += "----|-------------|-----\n"
			for k, v := range arg_desc.Fields {
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

	return strings.Replace(strings.Join(result, "\n"), "vfilter.Any", "Any", -1)
}

func formatFunctions(
	scope *vfilter.Scope,
	info *vfilter.ScopeInformation,
	type_map *vfilter.TypeMap) string {
	records := make(map[string]string)
	names := []string{}

	for _, item := range info.Functions {
		record := fmt.Sprintf("## %s\n\n%s\n\n", item.Name, item.Doc)
		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			record += "Arg | Description | Type\n"
			record += "----|-------------|-----\n"
			for k, v := range arg_desc.Fields {
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

	return strings.Replace(strings.Join(result, "\n"), "vfilter.Any", "Any", -1)
}

func doVQLList() {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	type_map := vfilter.NewTypeMap()
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

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case vql_info_list.FullCommand():
			doVQLList()
		default:
			return false
		}
		return true
	})
}
