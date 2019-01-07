package main

import (
	"fmt"
	"strings"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	vql_info      = app.Command("vql", "Show information about the VQL subsystem")
	vql_info_list = vql_info.Command("list", "Print all VQL plugins and functions")
)

func doVQLList() {
	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	type_map := vfilter.NewTypeMap()
	info := scope.Describe(type_map)

	fmt.Println("VQL Functions:")
	for _, item := range info.Functions {
		fmt.Printf("%s: %s\n", item.Name, item.Doc)
		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			fmt.Printf("  Args:\n")
			for k, v := range arg_desc.Fields {
				target := "type " + v.Target
				if v.Repeated {
					target = " list of " + target
				}

				required := ""
				if strings.Contains(v.Tag, "required") {
					required = "(required)"
				}
				fmt.Printf("     %s:  %s %s\n", k, target, required)
			}
		}
		fmt.Println()
	}

	fmt.Println("VQL Plugins:")
	for _, item := range info.Plugins {
		fmt.Printf("%s: %s\n", item.Name, item.Doc)
		arg_desc, pres := type_map.Get(scope, item.ArgType)
		if pres {
			fmt.Printf("  Args:\n")
			for k, v := range arg_desc.Fields {
				target := "type " + v.Target
				if v.Repeated {
					target = " list of " + target
				}

				required := ""
				if strings.Contains(v.Tag, "required") {
					required = "(required)"
				}
				fmt.Printf("     %s:  %s %s\n", k, target, required)
			}
		}
		fmt.Println()
	}
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
