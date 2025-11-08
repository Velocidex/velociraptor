package vql

import (
	"context"
	"runtime"

	"github.com/Velocidex/ordereddict"
	"gopkg.in/yaml.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

const (
	LOG_TAG = "unimplemented_log"
)

type UnimplementedFunction struct {
	Name      string
	Platforms []string
}

func (self *UnimplementedFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	DeduplicatedLog(scope, self.Name,
		"VQL Function %v() is not implemented for this architecture (%v). It is only available for the following platforms %v",
		self.Name, GetMyPlatform(), self.Platforms)

	return vfilter.Null{}
}

func (self *UnimplementedFunction) Copy() types.FunctionInterface {
	return &UnimplementedFunction{
		Name:      self.Name,
		Platforms: self.Platforms,
	}
}

func (self *UnimplementedFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: self.Name,
		Doc:  "Unimplemented Function",

		// Negative version means this plugin really does not exist.
		Version: -1,
	}
}

type RejectedFunction struct {
	UnimplementedFunction
}

func (self *RejectedFunction) Copy() types.FunctionInterface {
	return &RejectedFunction{
		UnimplementedFunction{
			Name:      self.Name,
			Platforms: self.Platforms,
		}}
}

func (self *RejectedFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	DeduplicatedLog(scope, self.Name,
		"VQL Function %v() has been blocked in the configuration. Please update the configuration file to allow it.",
		self.Name)

	return vfilter.Null{}
}

func NewRejectedFunction(name string) *RejectedFunction {
	return &RejectedFunction{UnimplementedFunction{
		Name: name,
	}}
}

type UnimplementedPlugin struct {
	Name      string
	Platforms []string
}

func (self *UnimplementedPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	DeduplicatedLog(scope, self.Name,
		"VQL Plugin %v() is not implemented for this architecture (%v). It is only available for the following platforms %v",
		self.Name, GetMyPlatform(), self.Platforms)

	close(output_chan)

	return output_chan
}

func (self *UnimplementedPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: self.Name,
		Doc:  "Unimplemented Plugin",

		// Negative version means this plugin really does not
		// exist. Version 0 is the default version for new plugins.
		Version: -1,
	}
}

type RejectedPlugin struct {
	UnimplementedPlugin
}

func (self *RejectedPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	DeduplicatedLog(scope, self.Name,
		"VQL Plugin %v() has been blocked in the configuration. Please update the configuration file to allow it.",
		self.Name)

	close(output_chan)

	return output_chan
}

func NewRejectedPlugin(name string) *RejectedPlugin {
	return &RejectedPlugin{UnimplementedPlugin{
		Name: name,
	}}
}

func _GetMyPlatform() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

func DeduplicatedLog(scope vfilter.Scope, key string, fmt string, args ...interface{}) {
	log_cache_any := CacheGet(scope, LOG_TAG)
	log_cache, ok := log_cache_any.(map[string]bool)
	if !ok {
		log_cache = make(map[string]bool)
	}

	_, ok = log_cache[key]
	if !ok {
		scope.Log(fmt, args...)
	}

	log_cache[key] = true
	CacheSet(scope, LOG_TAG, log_cache)
}

// Add unimplemented stubs for any plugins that are not available on
// this platform. This is normally only called once when the global
// scope is created.
func InstallUnimplemented(scope vfilter.Scope) {
	platform := GetMyPlatform()

	switch platform {
	// We only add metadata for some platforms so we can only really
	// apply this sometimes.
	case "linux_amd64_cgo",
		"linux_amd64_nocgo",
		"windows_386_cgo",
		"windows_386_nocgo",
		"windows_amd64_cgo",
		"windows_amd64_nocgo",
		"darwin_amd64_nocgo",
		"darwin_amd64_cgo":

		data, err := utils.GzipUncompress(assets.FileDocsReferencesVqlYaml)
		if err != nil {
			scope.Log("InstallUnimplemented: %v", err)
			return
		}

		result := []*api_proto.Completion{}
		err = yaml.Unmarshal(data, &result)
		if err != nil {
			scope.Log("InstallUnimplemented: %v", err)
			return
		}

		for _, item := range result {
			// Add a placeholder
			if item.Type == "Plugin" {
				// Skip plugins that are already supported.
				_, ok := scope.GetPlugin(item.Name)
				if !ok {
					RegisterPlugin(&UnimplementedPlugin{
						Name:      item.Name,
						Platforms: item.Platforms,
					})
				}

			} else if item.Type == "Function" {
				_, ok := scope.GetFunction(item.Name)
				if !ok {
					RegisterFunction(&UnimplementedFunction{
						Name:      item.Name,
						Platforms: item.Platforms,
					})
				}
			}
		}

	}
}
