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
/*

  The VQL subsystem allows for collecting host based state information
  using Velocidex Query Language (VQL) queries.

  The primary use case for Velociraptor is for incident
  response/detection and host based inventory management.
*/

package vql

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/vfilter"
)

var (
	exportedPlugins      = make(map[string]vfilter.PluginGeneratorInterface)
	exportedProtocolImpl []vfilter.Any
	exportedFunctions    = make(map[string]vfilter.FunctionInterface)

	scopeCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vql_make_scope",
		Help: "Total number of Scope objects constructed.",
	})
)

func OverridePlugin(plugin vfilter.PluginGeneratorInterface) {
	name := plugin.Info(nil, nil).Name
	exportedPlugins[name] = plugin
}

func RegisterPlugin(plugin vfilter.PluginGeneratorInterface) {
	name := plugin.Info(nil, nil).Name
	_, pres := exportedPlugins[name]
	if pres {
		panic("Multiple plugins defined")
	}

	exportedPlugins[name] = plugin
}

func RegisterFunction(plugin vfilter.FunctionInterface) {
	name := plugin.Info(nil, nil).Name
	_, pres := exportedFunctions[name]
	if pres {
		panic("Multiple plugins defined")
	}

	exportedFunctions[name] = plugin
}

func RegisterProtocol(plugin vfilter.Any) {
	exportedProtocolImpl = append(exportedProtocolImpl, plugin)
}

func GetFunction(name string) (vfilter.FunctionInterface, bool) {
	res, pres := exportedFunctions[name]
	return res, pres
}

var (
	mu sync.Mutex

	// Instead of building the scope from scratch each time, use a
	// global scope and prepare any other scopes from it.
	globalScope vfilter.Scope
)

func _makeRootScope() vfilter.Scope {
	mu.Lock()
	defer mu.Unlock()

	if globalScope == nil {
		globalScope = MakeNewScope()
	}

	return globalScope.NewScope()
}

func MakeScope() vfilter.Scope {
	return _makeRootScope()
}

func GetRootScope(scope vfilter.Scope) vfilter.Scope {
	root_any, pres := scope.Resolve(constants.SCOPE_ROOT)
	if pres {
		root, ok := root_any.(vfilter.Scope)
		if ok {
			return root
		}
	}
	return scope
}

// MakeNewScope makes a new scope from scratch. You do not need to use
// this! use MakeScope() above which is much faster.
func MakeNewScope() vfilter.Scope {
	scopeCounter.Inc()

	result := vfilter.NewScope()
	for _, plugin := range exportedPlugins {
		result.AppendPlugins(plugin)
	}

	for _, protocol := range exportedProtocolImpl {
		result.AddProtocolImpl(protocol)
	}

	for _, function := range exportedFunctions {
		result.AppendFunctions(function)
	}

	return result
}
