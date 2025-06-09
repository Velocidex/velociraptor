package tools

import (
	"context"
	"errors"
	"reflect"
	"runtime/debug"

	"github.com/Velocidex/ordereddict"
	"github.com/robertkrimen/otto"
	_ "github.com/robertkrimen/otto/underscore"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var halt = errors.New("Halt")

type JSCompileArgs struct {
	JS  string `vfilter:"required,field=js,doc=The body of the javascript code."`
	Key string `vfilter:"optional,field=key,doc=If set use this key to cache the JS VM."`
}

func logIfPanic(scope vfilter.Scope) {
	err := recover()
	if err == halt {
		return
	}

	if err != nil {
		scope.Log("PANIC %v: %v\n", err, string(debug.Stack()))
	}
}

type JSCompile struct{}

func getVM(ctx context.Context,
	scope vfilter.Scope,
	key string) *otto.Otto {
	if key == "" {
		key = "__jscontext"
	}

	vm, ok := vql_subsystem.CacheGet(scope, key).(*otto.Otto)
	if !ok {
		vm = otto.New()
		vm.Interrupt = make(chan func(), 1)
		go func() {
			<-ctx.Done()
			vm.Interrupt <- func() {
				panic(halt)
			}
		}()
		vql_subsystem.CacheSet(scope, key, vm)
	}

	return vm
}

func (self *JSCompile) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "js", args)()

	arg := &JSCompileArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("js: %s", err.Error())
		return vfilter.Null{}
	}

	defer logIfPanic(scope)

	vm := getVM(ctx, scope, arg.Key)
	_, err = vm.Run(arg.JS)
	if err != nil {
		scope.Log("js: %s", err.Error())
		return vfilter.Null{}
	}

	return vfilter.Null{}
}

func (self JSCompile) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "js",
		Doc:     "Compile and run javascript code.",
		ArgType: type_map.AddType(scope, &JSCompileArgs{}),
	}
}

type JSCallArgs struct {
	Func string      `vfilter:"required,field=func,doc=JS function to call."`
	Args vfilter.Any `vfilter:"optional,field=args,doc=Positional args for the function."`
	Key  string      `vfilter:"optional,field=key,doc=If set use this key to cache the JS VM."`
}

type JSCall struct{}

func (self *JSCall) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "js_call", args)()

	arg := &JSCallArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("js_call: %s", err.Error())
		return vfilter.Null{}
	}

	defer logIfPanic(scope)

	var call_args []interface{}

	slice := reflect.ValueOf(arg.Args)

	if arg.Args == nil {
		call_args = nil
	} else if slice.Type().Kind() != reflect.Slice {
		call_args = append(call_args, arg.Args)
	} else {
		for i := 0; i < slice.Len(); i++ {
			value := slice.Index(i).Interface()
			call_args = append(call_args, value)
		}
	}

	vm := getVM(ctx, scope, arg.Key)
	value, err := vm.Call(arg.Func, nil, call_args...)
	if err != nil {
		scope.Log("js_call: %s", err.Error())
		return vfilter.Null{}
	}

	result, _ := value.Export()
	if result == nil {
		result = vfilter.Null{}
	}
	return result
}

func (self JSCall) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "js_call",
		Doc:     "Compile and run javascript code.",
		ArgType: type_map.AddType(scope, &JSCallArgs{}),
	}
}

type JSSetArgs struct {
	Var   string      `vfilter:"required,field=var,doc=The variable to set inside the JS VM."`
	Value vfilter.Any `vfilter:"required,field=value,doc=The value to set inside the VM."`
	Key   string      `vfilter:"optional,field=key,doc=If set use this key to cache the JS VM."`
}

type JSSet struct{}

func (self *JSSet) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "js_set", args)()

	arg := &JSSetArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("js_set: %s", err.Error())
		return vfilter.Null{}
	}

	defer logIfPanic(scope)

	vm := getVM(ctx, scope, arg.Key)

	reflected := reflect.ValueOf(arg.Value)

	// If this isn't a slice, feed the data as is.
	if reflected.Type().Kind() != reflect.Slice {
		err = vm.Set(arg.Var, reflected.Interface())
	} else {
		// If it is a slice convert to array of interfaces and push
		var var_val []interface{}
		for i := 0; i < reflected.Len(); i++ {
			value := reflected.Index(i).Interface()
			var_val = append(var_val, value)
		}
		err = vm.Set(arg.Var, var_val)

	}

	if err != nil {
		scope.Log("js_set: %s", err.Error())
		return vfilter.Null{}
	}

	return vfilter.Null{}
}

func (self JSSet) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "js_set",
		Doc:     "Set a variables value in the JS VM.",
		ArgType: type_map.AddType(scope, &JSSetArgs{}),
	}
}

type JSGetArgs struct {
	Var string `vfilter:"required,field=var,doc=The variable to get from the JS VM."`
	Key string `vfilter:"optional,field=key,doc=If set use this key to cache the JS VM."`
}

type JSGet struct{}

func (self *JSGet) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "js_get", args)()

	arg := &JSGetArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("js_get: %s", err.Error())
		return vfilter.Null{}
	}

	defer logIfPanic(scope)

	vm := getVM(ctx, scope, arg.Key)

	otto_val, err := vm.Get(arg.Var)
	if err != nil {
		scope.Log("js_get: %s", err.Error())
		return vfilter.Null{}
	}
	value, err := otto_val.Export()
	if err != nil {
		scope.Log("js_get: %s", err.Error())
		return vfilter.Null{}
	}

	return value
}

func (self JSGet) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "js_get",
		Doc:     "Get a variable's value from the JS VM.",
		ArgType: type_map.AddType(scope, &JSGetArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&JSCall{})
	vql_subsystem.RegisterFunction(&JSCompile{})
	vql_subsystem.RegisterFunction(&JSSet{})
	vql_subsystem.RegisterFunction(&JSGet{})
}
