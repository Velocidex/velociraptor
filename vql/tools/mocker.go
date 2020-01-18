package tools

import (
	"context"
	"reflect"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type MockerPlugin struct {
	result vfilter.Any
	name   string
}

func (self MockerPlugin) Call(ctx context.Context,
	scope *vfilter.Scope, args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		a_value := reflect.Indirect(reflect.ValueOf(self.result))
		a_type := a_value.Type()

		if a_type.Kind() == reflect.Slice {
			for i := 0; i < a_value.Len(); i++ {
				element := a_value.Index(i).Interface()
				output_chan <- element
			}

		} else {
			output_chan <- self.result
		}
	}()
	return output_chan
}

func (self *MockerPlugin) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: self.name,
	}
}

// Replace a VQL function with a mock
type MockerFunctionArgs struct {
	Plugin   string      `vfilter:"optional,field=plugin,doc=The plugin to mock"`
	Artifact vfilter.Any `vfilter:"optional,field=artifact,doc=The plugin to mock"`
	Results  vfilter.Any `vfilter:"required,field=results,doc=The result to return"`
}

type MockFunction struct{}

func (self *MockFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &MockerFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("mock: %s", err.Error())
		return vfilter.Null{}
	}

	rt := reflect.TypeOf(arg.Results)
	if rt == nil || rt.Kind() != reflect.Slice {
		scope.Log("mock: results should be a list")
		return vfilter.Null{}
	}

	rows := []vfilter.Row{}
	value := reflect.ValueOf(arg.Results)
	for i := 0; i < value.Len(); i++ {
		rows = append(rows, value.Index(i).Interface())
	}

	if arg.Plugin != "" {
		scope.AppendPlugins(&MockerPlugin{result: arg.Results, name: arg.Plugin})
	} else if arg.Artifact != nil {
		artifact_plugin, ok := arg.Artifact.(*artifacts.ArtifactRepositoryPlugin)
		if !ok {
			scope.Log("mock: artifact is not defined")
			return vfilter.Null{}
		}
		artifact_plugin.SetMock(rows)
	} else {
		scope.Log("mock: either plugin or artifact should be specified")
		return vfilter.Null{}
	}

	return vfilter.Null{}
}

func (self MockFunction) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "mock",
		Doc:     "Mock a plugin.",
		ArgType: type_map.AddType(scope, &MockerFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&MockFunction{})
}
