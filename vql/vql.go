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
/*

  The VQL subsystem allows for collecting host based state information
  using Velocidex Query Language (VQL) queries.

  The primary use case for Velociraptor is for incident
  response/detection and host based inventory management.
*/

package vql

import (
	"fmt"
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

// Used when we deliberately want to override a registered plugin.
func OverridePlugin(plugin vfilter.PluginGeneratorInterface) {
	mu.Lock()
	defer mu.Unlock()

	name := plugin.Info(nil, nil).Name
	exportedPlugins[name] = plugin

	resetGlobalScopeCache()
}

// Used when we deliberately want to override a registered function.
func OverrideFunction(function vfilter.FunctionInterface) {
	mu.Lock()
	defer mu.Unlock()

	name := function.Info(nil, nil).Name
	exportedFunctions[name] = function

	resetGlobalScopeCache()
}

func RegisterPlugin(plugin vfilter.PluginGeneratorInterface) {
	mu.Lock()
	defer mu.Unlock()

	name := plugin.Info(nil, nil).Name
	_, pres := exportedPlugins[name]
	if pres {
		panic(fmt.Sprintf("Multiple plugins defined: %v", name))
	}

	exportedPlugins[name] = plugin

	resetGlobalScopeCache()
}

func RegisterFunction(plugin vfilter.FunctionInterface) {
	mu.Lock()
	defer mu.Unlock()

	name := plugin.Info(nil, nil).Name
	_, pres := exportedFunctions[name]
	if pres {
		panic(fmt.Sprintf("Multiple vql functions defined: %v", name))
	}

	exportedFunctions[name] = plugin

	resetGlobalScopeCache()
}

func RegisterProtocol(plugin vfilter.Any) {
	mu.Lock()
	defer mu.Unlock()

	exportedProtocolImpl = append(exportedProtocolImpl, plugin)

	resetGlobalScopeCache()
}

func EnforceVQLAllowList(
	allowed_plugins []string, allowed_functions []string,
	deny_plugins []string, deny_functions []string) error {

	mu.Lock()
	defer mu.Unlock()

	base_scope := vfilter.NewScope()

	if len(allowed_plugins) > 0 {
		new_exported_plugins := make(map[string]vfilter.PluginGeneratorInterface)
		for _, plugin_name := range allowed_plugins {
			impl, ok := exportedPlugins[plugin_name]
			if !ok {
				// Maybe this is provided by the base scope.
				impl, ok = base_scope.GetPlugin(plugin_name)
				if !ok {
					// Cant add it - just insert a stub
					impl = NewRejectedPlugin(plugin_name)
				}
			}
			new_exported_plugins[plugin_name] = impl
		}

		// Now install rejected plugins in place of all the existing
		// plugins so we can emit the correct error message.
		for k, v := range exportedPlugins {
			_, pres := new_exported_plugins[k]
			if pres {
				continue
			}

			_, pres = v.(*UnimplementedPlugin)
			if pres {
				continue
			}
			new_exported_plugins[k] = NewRejectedPlugin(k)
		}

		exportedPlugins = new_exported_plugins
	}

	for _, deny := range deny_plugins {
		exportedPlugins[deny] = NewRejectedPlugin(deny)
	}

	if len(allowed_functions) > 0 {
		new_exported_functions := make(map[string]vfilter.FunctionInterface)
		for _, func_name := range allowed_functions {
			impl, ok := exportedFunctions[func_name]
			if !ok {
				// Maybe this is provided by the base scope.
				impl, ok = base_scope.GetFunction(func_name)
				if !ok {
					impl = NewRejectedFunction(func_name)
				}
			}
			new_exported_functions[func_name] = impl
		}

		// Now install rejected plugins in place of all the existing
		// plugins so we can emit the correct error message.
		for k, v := range exportedFunctions {
			_, pres := new_exported_functions[k]
			if pres {
				continue
			}

			_, pres = v.(*UnimplementedFunction)
			if pres {
				continue
			}
			new_exported_functions[k] = NewRejectedFunction(k)
		}

		exportedFunctions = new_exported_functions
	}

	for _, deny := range deny_functions {
		exportedFunctions[deny] = NewRejectedFunction(deny)
	}

	// Reset the global scope so we will be forced to recreate it.
	resetGlobalScopeCache()

	return nil
}

var (
	mu sync.Mutex

	// Instead of building the scope from scratch each time, use a
	// global scope and prepare any other scopes from it.
	globalScope vfilter.Scope
)

func MakeScope() vfilter.Scope {
	mu.Lock()
	defer mu.Unlock()

	return _makeRootScope()
}

func _makeRootScope() vfilter.Scope {
	if globalScope == nil {
		globalScope = _MakeNewScope()
	}

	return globalScope.NewScope()
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
	mu.Lock()
	defer mu.Unlock()

	return _MakeNewScope()
}

func _MakeNewScope() vfilter.Scope {
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

// Used in tests to flush the global scope - needed **after**
// overriding plugins with OverridePlugin, OverrideFunction etc.
func ResetGlobalScopeCache() {
	mu.Lock()
	defer mu.Unlock()

	globalScope = nil
}

func resetGlobalScopeCache() {
	globalScope = nil
}
