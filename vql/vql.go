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
	"www.velocidex.com/golang/vfilter"
)

var (
	exportedPlugins      []vfilter.PluginGeneratorInterface
	exportedProtocolImpl []vfilter.Any
	exportedFunctions    []vfilter.FunctionInterface
)

func RegisterPlugin(plugin vfilter.PluginGeneratorInterface) {
	exportedPlugins = append(exportedPlugins, plugin)
}

func RegisterFunction(plugin vfilter.FunctionInterface) {
	exportedFunctions = append(exportedFunctions, plugin)
}

func RegisterProtocol(plugin vfilter.Any) {
	exportedProtocolImpl = append(exportedProtocolImpl, plugin)
}

func MakeScope() *vfilter.Scope {
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

	result.SetContext("", "")

	return result
}
