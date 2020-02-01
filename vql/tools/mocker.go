package tools

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _MockingScopeContext struct {
	plugins   []*MockerPlugin
	functions []*MockerFunction
}

func (self *_MockingScopeContext) AddPlugin(pl *MockerPlugin) {
	self.plugins = append(self.plugins, pl)
}

func (self *_MockingScopeContext) AddFunction(pl *MockerFunction) {
	self.functions = append(self.functions, pl)
}

func (self *_MockingScopeContext) GetPlugin(name string) *MockerPlugin {
	for _, pl := range self.plugins {
		if name == pl.name {
			return pl
		}
	}

	return nil
}

func (self _MockingScopeContext) GetFunction(name string) *MockerFunction {
	for _, pl := range self.functions {
		if name == pl.name {
			return pl
		}
	}

	return nil
}

func (self *_MockingScopeContext) Reset() {
	for _, pl := range self.plugins {
		pl.ctx.call_count = 0
	}

	for _, pl := range self.functions {
		pl.ctx.call_count = 0
	}
}

type _MockerCtx struct {
	results []vfilter.Any
	args    []*ordereddict.Dict

	call_count int
}

type MockerPlugin struct {
	name string
	ctx  *_MockerCtx
}

func (self MockerPlugin) Call(ctx context.Context,
	scope *vfilter.Scope, args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		self.ctx.args = append(self.ctx.args, args)

		result := self.ctx.results[self.ctx.call_count%len(self.ctx.results)]
		self.ctx.call_count += 1

		a_value := reflect.Indirect(reflect.ValueOf(result))
		a_type := a_value.Type()

		if a_type.Kind() == reflect.Slice {
			for i := 0; i < a_value.Len(); i++ {
				element := a_value.Index(i).Interface()
				output_chan <- element
			}

		} else {
			output_chan <- result
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
	Function string      `vfilter:"optional,field=function,doc=The function to mock"`
	Artifact vfilter.Any `vfilter:"optional,field=artifact,doc=The artifact to mock"`
	Results  vfilter.Any `vfilter:"required,field=results,doc=The result to return"`
}

type MockerFunction struct {
	name string
	ctx  *_MockerCtx
}

func (self *MockerFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	result := self.ctx.results[self.ctx.call_count%len(self.ctx.results)]
	self.ctx.call_count += 1

	return result
}

func (self *MockerFunction) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: self.name,
	}
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

	rows := []vfilter.Row{}

	rt := reflect.TypeOf(arg.Results)
	if rt == nil {
		scope.Log("mock: results should be a list")
		return vfilter.Null{}
	}

	if rt.Kind() != reflect.Slice {
		rows = append(rows, arg.Results)
	} else {
		value := reflect.ValueOf(arg.Results)
		for i := 0; i < value.Len(); i++ {
			rows = append(rows, value.Index(i).Interface())
		}
	}

	var scope_context *_MockingScopeContext

	scope_context_any := scope.GetContext("_mock_")
	if scope_context_any != nil {
		scope_context = scope_context_any.(*_MockingScopeContext)
	} else {
		scope_context = &_MockingScopeContext{}
		scope.SetContext("_mock_", scope_context)
	}

	if arg.Plugin != "" {
		mock_plugin := scope_context.GetPlugin(arg.Plugin)
		if mock_plugin == nil {
			mock_plugin = &MockerPlugin{name: arg.Plugin, ctx: &_MockerCtx{}}
			scope_context.AddPlugin(mock_plugin)
		}
		mock_plugin.ctx.results = append(mock_plugin.ctx.results, arg.Results)

		scope.AppendPlugins(mock_plugin)

	} else if arg.Function != "" {
		mock_plugin := scope_context.GetFunction(arg.Function)
		if mock_plugin == nil {
			mock_plugin = &MockerFunction{name: arg.Function, ctx: &_MockerCtx{}}
			scope_context.AddFunction(mock_plugin)
		}

		mock_plugin.ctx.results = append(mock_plugin.ctx.results, arg.Results)
		scope.AppendFunctions(mock_plugin)

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

type MockCheckArgs struct {
	Plugin        string `vfilter:"optional,field=plugin,doc=The plugin to mock"`
	Function      string `vfilter:"optional,field=function,doc=The function to mock"`
	ExpectedCalls int    `vfilter:"optional,field=expected_calls,doc=How many times plugin should be called"`
	Clear         bool   `vfilter:"optional,field=clear,doc=This call will clear previous mocks for this plugin"`
}

type MockCheckFunction struct{}

func (self *MockCheckFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &MockCheckArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("mock_check: %s", err.Error())
		return vfilter.Null{}
	}

	var scope_context *_MockingScopeContext

	scope_context_any := scope.GetContext("_mock_")
	if scope_context_any != nil {
		scope_context = scope_context_any.(*_MockingScopeContext)
	} else {
		scope_context = &_MockingScopeContext{}
		scope.SetContext("_mock_", scope_context)
	}

	if arg.Plugin != "" {
		mock_plugin := scope_context.GetPlugin(arg.Plugin)
		if mock_plugin == nil {
			scope.Log("mock_check: %s does not appear to be mocked", arg.Plugin)
			return vfilter.Null{}
		}

		if arg.ExpectedCalls != mock_plugin.ctx.call_count {
			return ordereddict.NewDict().Set(
				"Error", fmt.Sprintf(
					"Mock plugin %v should be called %v times but was called %v times",
					arg.Plugin, arg.ExpectedCalls,
					mock_plugin.ctx.call_count))
		}
		mock_plugin.ctx.call_count = 0
	}

	if arg.Function != "" {
		mock_plugin := scope_context.GetFunction(arg.Function)
		if mock_plugin == nil {
			scope.Log("mock_check: %s does not appear to be mocked", arg.Function)
			return vfilter.Null{}
		}

		if arg.ExpectedCalls != mock_plugin.ctx.call_count {
			return ordereddict.NewDict().Set(
				"Error", fmt.Sprintf(
					"Mock function %v should be called %v times but was called %v times",
					arg.Function, arg.ExpectedCalls,
					mock_plugin.ctx.call_count))
		}
		mock_plugin.ctx.call_count = 0
	}

	return ordereddict.NewDict().Set("Error", "OK")
}

func (self MockCheckFunction) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "mock_check",
		Doc:     "Check expectations on a mock.",
		ArgType: type_map.AddType(scope, &MockCheckArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&MockFunction{})
	vql_subsystem.RegisterFunction(&MockCheckFunction{})
}
