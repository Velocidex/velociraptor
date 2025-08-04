package remapping

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type MockingScopeContext struct {
	plugins   []*MockerPlugin
	functions []*MockerFunction
}

func (self *MockingScopeContext) AddPlugin(pl *MockerPlugin) {
	self.plugins = append(self.plugins, pl)
}

func (self *MockingScopeContext) AddFunction(pl *MockerFunction) {
	self.functions = append(self.functions, pl)
}

func (self *MockingScopeContext) GetPlugin(name string) *MockerPlugin {
	for _, pl := range self.plugins {
		if name == pl.name {
			return pl
		}
	}

	return nil
}

func (self MockingScopeContext) GetFunction(name string) *MockerFunction {
	for _, pl := range self.functions {
		if name == pl.name {
			return pl
		}
	}

	return nil
}

func (self *MockingScopeContext) Reset() {
	for _, pl := range self.plugins {
		pl.ctx.call_count = 0
		pl.ctx.recordings = nil
		pl.ctx.results = nil
	}

	for _, pl := range self.functions {
		pl.ctx.call_count = 0
	}
}

type _MockerCtx struct {
	mu         sync.Mutex
	results    []types.Any
	recordings []*ordereddict.Dict

	call_count int
}

type MockerPlugin struct {
	name string
	ctx  *_MockerCtx
}

func NewMockerPlugin(name string, results []types.Any) *MockerPlugin {
	result := &MockerPlugin{
		name: name,
		ctx:  &_MockerCtx{},
	}

	for _, item := range results {
		result.ctx.results = append(result.ctx.results, item)
	}
	return result
}

func (self MockerPlugin) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) <-chan types.Row {
	output_chan := make(chan types.Row)
	go func() {
		defer close(output_chan)

		self.ctx.mu.Lock()
		if len(self.ctx.results) == 0 {
			self.ctx.mu.Unlock()
			return
		}
		result := self.ctx.results[self.ctx.call_count%len(self.ctx.results)]
		self.ctx.call_count += 1
		self.ctx.recordings = append(self.ctx.recordings, args)
		self.ctx.mu.Unlock()

		a_value := reflect.Indirect(reflect.ValueOf(result))
		a_type := a_value.Type()

		// It is a multi-call mock. The array represents an entire
		// call.
		if a_type.Kind() == reflect.Slice {
			for i := 0; i < a_value.Len(); i++ {
				element := a_value.Index(i).Interface()
				select {
				case <-ctx.Done():
					return
				case output_chan <- element:
				}
			}

			// It is a multi-row mock of a single call - dump all
			// items into rows.
		} else {
			select {
			case <-ctx.Done():
				return
			case output_chan <- result:
			}
		}
	}()
	return output_chan
}

func (self *MockerPlugin) Info(scope types.Scope,
	type_map *types.TypeMap) *types.PluginInfo {
	return &types.PluginInfo{
		Name: self.name,
	}
}

// Replace a VQL function with a mock
type MockerFunctionArgs struct {
	Plugin   string         `vfilter:"optional,field=plugin,doc=The plugin to mock"`
	Function string         `vfilter:"optional,field=function,doc=The function to mock"`
	Artifact types.Any      `vfilter:"optional,field=artifact,doc=The artifact to mock"`
	Results  types.LazyExpr `vfilter:"required,field=results,doc=The result to return"`
}

type MockerFunction struct {
	name string
	ctx  *_MockerCtx
}

func (self *MockerFunction) Copy() types.FunctionInterface {
	return &MockerFunction{
		name: self.name,
		ctx:  self.ctx,
	}
}

func (self *MockerFunction) Call(ctx context.Context,
	scope types.Scope,
	args *ordereddict.Dict) types.Any {

	self.ctx.mu.Lock()
	result := self.ctx.results[self.ctx.call_count%len(self.ctx.results)]
	self.ctx.call_count += 1
	self.ctx.mu.Unlock()

	return result
}

func (self *MockerFunction) Info(scope types.Scope,
	type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{
		Name: self.name,
	}
}

func NewMockerFunction(name string, result []types.Any) *MockerFunction {
	return &MockerFunction{
		name: name,
		ctx: &_MockerCtx{
			results: result,
		},
	}
}

type MockFunction struct{}

func (self *MockFunction) Call(ctx context.Context,
	scope types.Scope,
	args *ordereddict.Dict) types.Any {

	arg := &MockerFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("mock: %s", err.Error())
		return types.Null{}
	}

	results := arg.Results.Reduce(ctx)

	results_query, ok := results.(types.StoredQuery)
	if ok {
		results = types.Materialize(ctx, scope, results_query)
	}

	var plugin_results []types.Any

	rt := reflect.TypeOf(results)
	if rt == nil {
		scope.Log("mock: results should be a list")
		return types.Null{}
	}

	// This field can be a number of options:

	// 1. A single dict - same as a list of one dict - just emit a
	//    single dict as a row

	// 2. A list of lists of dicts: Each list of dicts is a separate
	//    plugin invocation.

	// 3. A list of dicts: Multiple rows to be emitted in each
	//    invocation.

	if rt.Kind() != reflect.Slice {
		plugin_results = append(plugin_results, results)
	} else {
		value := reflect.ValueOf(results)
		if value.Len() == 0 {
			scope.Log("mock: results should be a list")
			return types.Null{}
		}

		// First item is a list - this is a multi-call mock
		if reflect.TypeOf(value.Index(0).Interface()).Kind() == reflect.Slice {
			for i := 0; i < value.Len(); i++ {
				item := value.Index(i).Interface()
				item_lazy, ok := item.(types.LazyExpr)
				if ok {
					item = item_lazy.Reduce(ctx)
				}
				plugin_results = append(plugin_results, item)
			}

			// All items are rows in a single call.
		} else {
			var rows []types.Row
			for i := 0; i < value.Len(); i++ {
				item := value.Index(i).Interface()
				item_lazy, ok := item.(types.LazyExpr)
				if ok {
					item = item_lazy.Reduce(ctx)
				}
				rows = append(rows, item)
			}
			plugin_results = append(plugin_results, rows)
		}
	}

	scope_context, ok := GetMockContext(scope)
	if !ok {
		scope.Log("mock_check: Not running in test.")
		return types.Null{}
	}

	if arg.Plugin != "" {
		mock_plugin := scope_context.GetPlugin(arg.Plugin)
		if mock_plugin == nil {
			mock_plugin = &MockerPlugin{name: arg.Plugin, ctx: &_MockerCtx{}}
			scope_context.AddPlugin(mock_plugin)
		}
		mock_plugin.ctx.results = plugin_results
		scope.AppendPlugins(mock_plugin)

	} else if arg.Function != "" {
		mock_plugin := scope_context.GetFunction(arg.Function)
		if mock_plugin == nil {
			mock_plugin = NewMockerFunction(arg.Function, []types.Any{})
			scope_context.AddFunction(mock_plugin)
		}

		mock_plugin.ctx.results = plugin_results
		scope.AppendFunctions(mock_plugin)

	} else if arg.Artifact != nil {
		item := arg.Artifact
		item_lazy, ok := item.(types.LazyExpr)
		if ok {
			item = item_lazy.Reduce(ctx)
		}

		artifact_plugin, ok := item.(services.MockablePlugin)
		if !ok {
			scope.Log("mock: artifact parameter should be an artifact not %T", item)
			return types.Null{}
		}
		var rows []types.Row
		for _, i := range plugin_results {
			rows = append(rows, i)
		}
		artifact_plugin.SetMock(artifact_plugin.Name(), rows)
	} else {
		scope.Log("mock: either plugin or artifact should be specified")
		return types.Null{}
	}

	return types.Null{}
}

func (self MockFunction) Info(
	scope types.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
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
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &MockCheckArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("mock_check: %s", err.Error())
		return vfilter.Null{}
	}

	scope_context, ok := GetMockContext(scope)
	if !ok {
		scope.Log("mock_check: Not running in test.")
		return vfilter.Null{}
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
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "mock_check",
		Doc:     "Check expectations on a mock.",
		ArgType: type_map.AddType(scope, &MockCheckArgs{}),
	}
}

func GetMockContext(scope vfilter.Scope) (*MockingScopeContext, bool) {
	scope_mocker, pres := scope.Resolve(constants.SCOPE_MOCK)
	if !pres {
		return nil, false
	}

	mocker, ok := scope_mocker.(*MockingScopeContext)
	return mocker, ok
}

type MockReplayFunction struct{}

func (self *MockReplayFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &MockCheckArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("mock_check: %s", err.Error())
		return vfilter.Null{}
	}

	scope_context, ok := GetMockContext(scope)
	if !ok {
		scope.Log("mock_check: Not running in test.")
		return vfilter.Null{}
	}

	if arg.Plugin != "" {
		mock_plugin := scope_context.GetPlugin(arg.Plugin)
		if mock_plugin == nil {
			scope.Log("mock_check: %s does not appear to be mocked", arg.Plugin)
			return vfilter.Null{}
		}

		return mock_plugin.ctx.recordings
	}

	if arg.Function != "" {
		mock_plugin := scope_context.GetFunction(arg.Function)
		if mock_plugin == nil {
			scope.Log("mock_check: %s does not appear to be mocked", arg.Function)
			return vfilter.Null{}
		}

		return mock_plugin.ctx.recordings
	}

	return vfilter.Null{}
}

func (self MockReplayFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "mock_replay",
		Doc:     "Replay recorded calls on a mock.",
		ArgType: type_map.AddType(scope, &MockCheckArgs{}),
	}
}

type MockClearFunction struct{}

func (self *MockClearFunction) Call(ctx context.Context,
	scope vfilter.Scope, args *ordereddict.Dict) vfilter.Any {

	scope_context, ok := GetMockContext(scope)
	if !ok {
		scope.Log("mock_clear: Not running in test.")
		return vfilter.Null{}
	}

	scope_context.Reset()
	return vfilter.Null{}
}

func (self MockClearFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "mock_clear",
		Doc:  "Resets all mocks.",
	}
}

type MockedScope struct {
	vfilter.Scope

	mu      sync.Mutex
	plugins map[string]*MockerPlugin
}

func (self *MockedScope) GetPlugin(name string) (types.PluginGeneratorInterface, bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	mock, pres := self.plugins[name]
	if pres {
		return mock, true
	}

	return self.Scope.GetPlugin(name)
}

func NewMockScope(scope vfilter.Scope, plugins []*MockerPlugin) *MockedScope {
	res := &MockedScope{
		Scope:   scope,
		plugins: make(map[string]*MockerPlugin),
	}

	for _, p := range plugins {
		res.plugins[p.name] = p
	}
	return res
}

func init() {
	vql_subsystem.RegisterFunction(&MockFunction{})
	vql_subsystem.RegisterFunction(&MockCheckFunction{})
	vql_subsystem.RegisterFunction(&MockClearFunction{})
	vql_subsystem.RegisterFunction(&MockReplayFunction{})
}
