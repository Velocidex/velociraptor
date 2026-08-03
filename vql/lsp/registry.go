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

package lsp

import (
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

// Arg describes a single keyword argument accepted by a plugin or function.
type Arg struct {
	Name string
	Type string
}

// Callable describes a plugin or function registered in the VQL scope.
type Callable struct {
	Name        string
	Doc         string
	Type        string // "plugin" or "function"
	Args        []Arg
	IsAggregate bool
	FreeForm    bool
}

// Registry holds the introspection data about all plugins and functions
// available in the Velociraptor VQL scope.
//
// The registry is built once at server startup by evaluating the real VQL
// scope (which has every plugin and function registered) and describing it.
type Registry struct {
	plugins   map[string]*Callable
	functions map[string]*Callable
}

// BuildRegistry creates a Registry from a live VQL scope.
//
// The scope is only used to introspect the registered plugins and
// functions; it is closed by the caller.
func BuildRegistry(scope vfilter.Scope) *Registry {
	type_map := types.NewTypeMap()
	info := scope.Describe(type_map)

	result := &Registry{
		plugins:   make(map[string]*Callable),
		functions: make(map[string]*Callable),
	}

	for _, item := range info.Plugins {
		callable := &Callable{
			Name:     item.Name,
			Doc:      item.Doc,
			Type:     "plugin",
			FreeForm: item.FreeFormArgs,
		}
		result.addArgs(scope, type_map, item.ArgType, callable)
		result.plugins[item.Name] = callable
	}

	for _, item := range info.Functions {
		callable := &Callable{
			Name:        item.Name,
			Doc:         item.Doc,
			Type:        "function",
			IsAggregate: item.IsAggregate,
			FreeForm:    item.FreeFormArgs,
		}
		result.addArgs(scope, type_map, item.ArgType, callable)
		result.functions[item.Name] = callable
	}

	return result
}

func (self *Registry) addArgs(
	scope vfilter.Scope, type_map *types.TypeMap,
	arg_type string, callable *Callable) {

	if arg_type == "" {
		return
	}
	type_desc, pres := type_map.Get(scope, arg_type)
	if !pres || type_desc == nil || type_desc.Fields == nil {
		return
	}

	for _, field := range type_desc.Fields.Items() {
		var type_ref string
		switch value := field.Value.(type) {
		case *types.TypeReference:
			type_ref = value.Target
		case string:
			type_ref = value
		}
		callable.Args = append(callable.Args, Arg{
			Name: field.Key,
			Type: type_ref,
		})
	}
}

// GetPlugin looks up a plugin by its full name.
func (self *Registry) GetPlugin(name string) (*Callable, bool) {
	callable, pres := self.plugins[name]
	return callable, pres
}

// GetFunction looks up a function by its full name.
func (self *Registry) GetFunction(name string) (*Callable, bool) {
	callable, pres := self.functions[name]
	return callable, pres
}

// AddArtifact registers an artifact as a pseudo-plugin. Artifacts are
// called from VQL like plugins, e.g. Artifact.Windows.Sys.Users().
//
// The artifact's declared parameters become the callable's args.
func (self *Registry) AddArtifact(
	name, description string, parameters []Arg) {
	self.plugins[name] = &Callable{
		Name: name,
		Doc:  description,
		Type: "artifact",
		Args: parameters,
	}
}

// AllCallables returns a merged view of all plugins and functions keyed by
// name. The registry is immutable after startup, so the returned map is
// safe for concurrent reads.
func (self *Registry) AllCallables() map[string]*Callable {
	result := make(map[string]*Callable, len(self.plugins)+len(self.functions))
	for name, callable := range self.plugins {
		result[name] = callable
	}
	for name, callable := range self.functions {
		result[name] = callable
	}
	return result
}
