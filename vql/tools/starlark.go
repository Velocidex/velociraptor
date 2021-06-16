package tools

import (
	"context"
	"errors"

	"reflect"

	"fmt"

	"github.com/Velocidex/ordereddict"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type StarlarkCompileArgs struct {
	Star string      `vfilter:"required,field=star,doc=The body of the starlark code."`
	Key  string      `vfilter:"optional,field=key,doc=If set use this key to cache the Starlark STHREAD."`
	Dict vfilter.Any `vfilter:"optional,field=dict,doc=Dictionary of values to feed into Starlark STHREAD"`
}

type StarlarkCompile struct{}

var ErrStarlarkConversion = errors.New("failed to convert Starlark data type")

// Convert starlark types to Golang and VQL types
// Code modified from https://github.com/cirruslabs/cirrus-cli/blob/master/pkg/larker/converthook.go
func starlarkValueAsInterface(value starlark.Value) (interface{}, error) {
	switch v := value.(type) {
	case *starlark.Function:
		return v, nil
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.Int:
		res, _ := v.Int64()

		return res, nil
	case starlark.Float:
		return float64(v), nil
	case starlark.String:
		return string(v), nil
	case *starlark.List:
		it := v.Iterate()
		defer it.Done()

		var listItem starlark.Value
		var result []interface{}

		for it.Next(&listItem) {
			listItemInterfaced, err := starlarkValueAsInterface(listItem)
			if err != nil {
				return nil, err
			}

			result = append(result, listItemInterfaced)
		}

		return result, nil

	case *starlark.Set:
		it := v.Iterate()
		defer it.Done()

		var listItem starlark.Value
		var result []interface{}

		for it.Next(&listItem) {
			listItemInterfaced, err := starlarkValueAsInterface(listItem)
			if err != nil {
				return nil, err
			}

			result = append(result, listItemInterfaced)
		}

		return result, nil

	case *starlark.Dict:
		result := ordereddict.NewDict()
		for _, item := range v.Items() {
			key := item[0]
			value := item[1]

			dictValueInterfaced, err := starlarkValueAsInterface(value)
			if err != nil {
				return nil, err
			}
			result.Set(key.String(), dictValueInterfaced)
		}

		return result, nil
	default:
		return nil, fmt.Errorf("%w: unsupported type %T", ErrStarlarkConversion, value)
	}
}

// Convert Golang and VQL types to Starlark Types
// Code modified from https://github.com/cirruslabs/cirrus-cli/blob/master/pkg/larker/converthook.go
func interfaceAsStarlarkValue(value interface{}) (starlark.Value, error) {
	switch v := value.(type) {
	case nil:
		return starlark.None, nil
	case *vfilter.VQL:
		return nil, nil
	case *types.Null:
		return starlark.None, nil
	case types.Null:
		return starlark.None, nil
	case bool:
		return starlark.Bool(v), nil
	case int:
		return starlark.MakeInt(v), nil
	case int32:
		// Convert int32 to int due to lack of int32 in starlark
		new_int := int(v)
		return starlark.MakeInt(new_int), nil
	case int64:
		return starlark.MakeInt64(v), nil
	case uint:
		return starlark.MakeUint(v), nil
	case uint32:
		// Convert uint32 to int due to lack of uint32 in starlark
		new_uint := uint(v)
		return starlark.MakeUint(new_uint), nil
	case uint64:
		return starlark.MakeUint64(v), nil
	case float32:
		return tryIntegerOrFloat(float64(v))
	case float64:
		return tryIntegerOrFloat(v)
	case string:
		return starlark.String(v), nil
	case []interface{}:
		result := starlark.NewList([]starlark.Value{})

		for _, item := range v {
			listValueStarlarked, err := interfaceAsStarlarkValue(item)
			if err != nil {
				return nil, err
			}

			if err := result.Append(listValueStarlarked); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrStarlarkConversion, err)
			}
		}

		return result, nil
	case []vfilter.Any:
		result := starlark.NewList([]starlark.Value{})

		for _, item := range v {
			listValueStarlarked, err := interfaceAsStarlarkValue(item)
			if err != nil {
				return nil, err
			}

			if err := result.Append(listValueStarlarked); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrStarlarkConversion, err)
			}
		}

		return result, nil
	case map[string]interface{}:
		result := map[string]starlark.Value{}

		for key, value := range v {
			mapValueStarlarked, err := interfaceAsStarlarkValue(value)
			if err != nil {
				return nil, err
			}

			result[key] = mapValueStarlarked
		}

		dict := starlarkstruct.FromStringDict(starlarkstruct.Default, result)

		return dict, nil
	case map[int32]interface{}:

		keyword_tuple := make([]starlark.Tuple, 0)
		for key, value := range v {
			mapValueStarlarked, err := interfaceAsStarlarkValue(value)
			if err != nil {
				return nil, err
			}
			new_val := int(key)
			keyword_tuple = append(keyword_tuple, starlark.Tuple{starlark.MakeInt(new_val), mapValueStarlarked})
		}

		dict := starlarkstruct.FromKeywords(starlarkstruct.Default, keyword_tuple)

		return dict, nil

	case *ordereddict.Dict:
		// Most of the time we're here
		result := map[string]starlark.Value{}

		// Convert data to JSON and back to make sure it's types starlark can understand
		data, err := v.MarshalJSON()
		if err != nil {
			return nil, err
		}
		new_dict := ordereddict.NewDict()
		err = new_dict.UnmarshalJSON(data)
		if err != nil {
			return nil, err
		}
		for key, value := range *new_dict.ToDict() {
			mapValueStarlarked, err := interfaceAsStarlarkValue(value)
			if err != nil {
				return nil, err
			}

			result[key] = mapValueStarlarked
		}

		dict := starlarkstruct.FromStringDict(starlarkstruct.Default, result)

		return dict, nil

	default:
		return nil, fmt.Errorf("%w: unsupported type %T", ErrStarlarkConversion, value)
	}
}

func tryIntegerOrFloat(floatValue float64) (starlark.Value, error) {
	if integerValue := int64(floatValue); floatValue == float64(integerValue) {
		return starlark.MakeInt64(integerValue), nil
	}

	return starlark.Float(floatValue), nil
}

// Get scope of compiled starlark context
func getCompiled(ctx context.Context,
	scope vfilter.Scope,
	key string, code string, globals vfilter.Any) (*ordereddict.Dict, error) {
	if key == "" {
		key = "__slcontext"
	}

	compiled_vars, ok := vql_subsystem.CacheGet(scope, key).(*ordereddict.Dict)
	if !ok {
		compiled_vars = ordereddict.NewDict()
		err := compileStarlark(code, globals, compiled_vars)
		if err != nil {
			return nil, err
		}
		vql_subsystem.CacheSet(scope, key, compiled_vars)
	}

	return compiled_vars, nil
}

// User doesn't actually specify extras, that's done in extractStarlarkFuncArgs
type StarlarkFuncArgs struct {
	Func    string      `vfilter:"required,field=func,doc=Starlark function to call."`
	Code    string      `vfilter:"required,field=code,doc=Starlark code to compile."`
	Args    vfilter.Any `vfilter:"optional,field=args,doc=Positional args for the function."`
	Kwargs  vfilter.Any `vfilter:"optional,field=kwargs,doc=KW args for the function."`
	Key     string      `vfilter:"optional,field=key,doc=If set use this key to cache the Starlark code."`
	Extras  vfilter.Any `vfilter:"optional,field=extras,doc="Extra Arguments to pass to Starlark code. Set implicitly"`
	Globals vfilter.Any `vfilter:"optional,field=globals,doc="Global Arguments to pass to Starlark code."`
}

type StarlarkFunc struct{}

func compileStarlark(code string, globals vfilter.Any, compiled_vars *ordereddict.Dict) error {
	sthread := &starlark.Thread{Name: "VQL Thread"}
	predeclared, err := interfaceAsStarlarkValue(globals)
	if err != nil {
		return err

	}
	stringdict := starlark.StringDict{}
	switch predeclared.(type) {
	case *starlarkstruct.Struct:
		predeclared.(*starlarkstruct.Struct).ToStringDict(stringdict)
	case starlark.NoneType: // Do nothing
	default:
		return errors.New("Invalid data type provided to globals!")
	}
	new_globals, err := starlark.ExecFile(sthread, "apparent/filename.star", code, stringdict)
	if err != nil {
		return err
	}
	for key, item := range new_globals {
		entry, err := starlarkValueAsInterface(item)
		if err != nil {
			return err
		}
		compiled_vars.Set(key, entry)
	}

	return nil
}

// Pull out all non-builtin args and stick them in their own dict
func extractStarlarkFuncArgs(args *ordereddict.Dict, ctx context.Context) *ordereddict.Dict {
	result := ordereddict.NewDict()
	extra_args := ordereddict.NewDict()
	for key, value := range *args.ToDict() {
		if key != "func" && key != "code" && key != "args" && key != "kwargs" && key != "key" && key != "globals" {
			switch t := value.(type) {
			case types.LazyExpr:
				extra_args.Set(key, t.Reduce(ctx))
			default:
				extra_args.Set(key, t)
			}
		} else {
			result.Set(key, value)
		}
	}
	result.Set("extras", extra_args)
	return result
}

// Turn dicts into starlark tuples to pass to starlark.Call
func makeKwargsTuple(kwargs *ordereddict.Dict) ([]starlark.Tuple, error) {
	starlark_args, err := interfaceAsStarlarkValue(kwargs)
	if err != nil {
		return nil, err
	}
	starlark_tuple := make([]starlark.Tuple, 0)
	new_string_dict := starlark.StringDict{}
	starlark_args.(*starlarkstruct.Struct).ToStringDict(new_string_dict)
	for key, value := range new_string_dict {
		starlark_tuple = append(starlark_tuple, starlark.Tuple{starlark.String(key), value})
	}
	return starlark_tuple, nil
}

func (self *StarlarkFunc) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	// extract extras
	new_dict := extractStarlarkFuncArgs(args, ctx)
	arg := StarlarkFuncArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, new_dict, &arg)
	if err != nil {
		scope.Log("starl_func: %s", err.Error())
		return vfilter.Null{}
	}

	defer logIfPanic(scope)

	// grab compiled code
	compiled_args, err := getCompiled(ctx, scope, arg.Key, arg.Code, arg.Globals)
	if err != nil {
		scope.Log("starl_func: %v", err)
		return vfilter.Null{}
	}

	// create new thread per call
	sthread := &starlark.Thread{Name: "VQL Thread"}
	sfunc, ok := compiled_args.Get(arg.Func)
	if !ok {
		scope.Log("starl_func: %s not found within starlark scope!", arg.Func)
		return vfilter.Null{}
	}

	// should be function, if not, throw error
	// use starl_var for non-function values
	inferred, ok := sfunc.(*starlark.Function)
	if !ok {
		scope.Log("starl_func: %s is not a function!", arg.Func)
		return vfilter.Null{}
	}

	// return value from starlark.Call
	var value starlark.Value
	// only allow args, kwargs, or extras, not all 3 at once for simplicity
	if arg.Args != nil {
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

		starlark_args, err := interfaceAsStarlarkValue(call_args)
		if err != nil {
			scope.Log("starl_func: %s", err.Error())
			return vfilter.Null{}
		}
		tuple := starlark.Tuple{starlark_args}
		value, err = starlark.Call(sthread, inferred, tuple, nil)
		if err != nil {
			scope.Log("starl_func: %s", err.Error())
			return vfilter.Null{}
		}
	} else if arg.Kwargs != nil {
		starlark_tuple, nil := makeKwargsTuple(arg.Kwargs.(*ordereddict.Dict))
		if err != nil {
			scope.Log("starl_func: %s", err.Error())
			return vfilter.Null{}
		}
		empty_tuple := starlark.Tuple{}
		value, err = starlark.Call(sthread, inferred, empty_tuple, starlark_tuple)
		if err != nil {
			scope.Log("starl_func: %s", err.Error())
			return vfilter.Null{}
		}

	} else if arg.Extras.(*ordereddict.Dict).Len() > 0 {
		starlark_tuple, nil := makeKwargsTuple(arg.Extras.(*ordereddict.Dict))
		if err != nil {
			scope.Log("starl_func: %s", err.Error())
			return vfilter.Null{}
		}
		empty_tuple := starlark.Tuple{}
		value, err = starlark.Call(sthread, inferred, empty_tuple, starlark_tuple)
		if err != nil {
			scope.Log("starl_func 410: %s", err.Error())
			return vfilter.Null{}
		}

	} else {
		value, err = starlark.Call(sthread, inferred, nil, nil)
		if err != nil {
			scope.Log("starl_func: %s", err.Error())
			return vfilter.Null{}
		}

	}

	// convert results from starlark values to Golang and VQL values
	result, err := starlarkValueAsInterface(value)
	if err != nil {
		scope.Log("sl: %v", err)
		return vfilter.Null{}
	}
	return result
}

func (self *StarlarkFunc) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "starl_func",
		Doc:     "Run Starlark Functions",
		ArgType: type_map.AddType(scope, &StarlarkFuncArgs{}),
	}
}

type StarlarkVarsArgs struct {
	Code    string      `vfilter:"required,field=code,doc=Starlark code to compile."`
	Vars     []string    `vfilter:"optional,field=vars,doc=Name of variables to return. Returns all if not specified."`
	Key     string      `vfilter:"optional,field=key,doc=If set use this key to cache the Starlark code."`
	Globals vfilter.Any `vfilter:"optional,field=globals,doc="Global Arguments to pass to Starlark code."`
}

type StarlarkVars struct{}
func (self *StarlarkVars) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := StarlarkVarsArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, &arg)
	if err != nil {
		scope.Log("starl_vars: %s", err.Error())
		return vfilter.Null{}
	}

	defer logIfPanic(scope)

	// grab compiled code
	compiled_args, err := getCompiled(ctx, scope, arg.Key, arg.Code, arg.Globals)
	if err != nil {
		scope.Log("starl_vars: %v", err)
		return vfilter.Null{}
	}
	results := ordereddict.NewDict()
	if len(arg.Vars) != 0 {
		for _,item := range arg.Vars {
			ret,ok := compiled_args.Get(item)
			if !ok {
				scope.Log("starl_vars: %s not found!", item)
				return vfilter.Null{}
			}
			results.Set(item, ret)
		}
	} else {
		return compiled_args
	}
	return results

}


func (self *StarlarkVars) Info(scope vfilter.Scope,
      type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "starl_vars",
		Doc:     "Retreive Starlark Variables",
		ArgType: type_map.AddType(scope, &StarlarkVarsArgs{}),
	}
}

func init() {
	// Must be set to allow recursion and starlark sets
	resolve.AllowSet = true
	resolve.AllowRecursion = true
	vql_subsystem.RegisterFunction(&StarlarkFunc{})
	vql_subsystem.RegisterFunction(&StarlarkVars{})
}
