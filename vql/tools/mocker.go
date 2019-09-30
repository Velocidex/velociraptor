package tools

import (
	"context"
	"reflect"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type MockerPlugin struct {
	result vfilter.Any
	name   string
}

func (self MockerPlugin) Call(ctx context.Context,
	scope *vfilter.Scope, args *vfilter.Dict) <-chan vfilter.Row {
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
	Plugin  string      `vfilter:"required,field=plugin,doc=The plugin to mock"`
	Results vfilter.Any `vfilter:"required,field=results,doc=The result to return"`
}

type MockFunction struct{}

func (self *MockFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {

	arg := &MockerFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("mock: %s", err.Error())
		return vfilter.Null{}
	}

	scope.AppendPlugins(&MockerPlugin{result: arg.Results, name: arg.Plugin})

	return vfilter.Null{}
}

func (self MockFunction) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "mock",
		Doc:     "Upload files to GCS.",
		ArgType: type_map.AddType(scope, &MockerFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&MockFunction{})
}
