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

	return result
}
