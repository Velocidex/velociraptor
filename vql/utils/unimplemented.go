package utils

import (
	"context"
	"runtime"

	"github.com/Velocidex/ordereddict"
	"gopkg.in/yaml.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type UnimplementedFunction struct {
	Name      string
	Platforms []string
}

func (self *UnimplementedFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	functions.DeduplicatedLog(ctx, scope,
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
	}
}

type UnimplementedPlugin struct {
	Name      string
	Platforms []string
}

func (self *UnimplementedPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	functions.DeduplicatedLog(ctx, scope,
		"VQL Plugin %v() is not implemented for this architecture (%v). It is only available for the following platforms %v",
		self.Name, GetMyPlatform(), self.Platforms)

	close(output_chan)

	return output_chan
}

func (self *UnimplementedPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: self.Name,
		Doc:  "Unimplemented Plugin",
	}
}

func _GetMyPlatform() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

// Add unimplemented stubs for any plugins that are not available on
// this platform.
func init() {
	platform := GetMyPlatform()

	switch platform {
	// We only add metadata for some platforms so we can only really
	// apply this sometimes.
	case "linux_amd64_cgo",
		"windows_386_cgo", "windows_amd64_cgo",
		"darwin_amd64_cgo":

		assets.InitOnce()
		data, err := assets.ReadFile("docs/references/vql.yaml")
		if err != nil {
			return
		}

		result := []*api_proto.Completion{}
		err = yaml.Unmarshal(data, &result)
		if err == nil {
			for _, item := range result {
				// Skip plugins that are already supported.
				if utils.InString(item.Platforms, platform) {
					continue
				}

				// Add a placeholder
				if item.Type == "Plugin" {
					vql_subsystem.RegisterPlugin(&UnimplementedPlugin{
						Name:      item.Name,
						Platforms: item.Platforms,
					})

				} else if item.Type == "Function" {
					vql_subsystem.RegisterFunction(&UnimplementedFunction{
						Name:      item.Name,
						Platforms: item.Platforms,
					})
				}
			}
		}

	}
}
