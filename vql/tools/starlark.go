package tools

import (
	"context"
	"errors"

	"reflect"

	"fmt"

	"github.com/Velocidex/ordereddict"
	"github.com/qri-io/starlib"
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

	case starlark.Tuple:
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

	case *starlarkstruct.Struct:
		result := ordereddict.NewDict()
		string_dict := starlark.StringDict{}
		v.ToStringDict(string_dict)
		for key, value := range string_dict {
			res, err := starlarkValueAsInterface(value)
			if err != nil {
				return nil, err
			}
			result.Set(key, res)
		}
		return result,nil
	default:
		return nil, fmt.Errorf("%w: unsupported type %T", ErrStarlarkConversion, value)
	}
}

// Convert Golang and VQL types to Starlark Types
// Code modified from https://github.com/cirruslabs/cirrus-cli/blob/master/pkg/larker/converthook.go
func interfaceAsStarlarkValue(value interface{}, ctx context.Context, scope vfilter.Scope) (starlark.Value, error) {
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
			listValueStarlarked, err := interfaceAsStarlarkValue(item, ctx, scope)
			if err != nil {
				return nil, err
			}

			if err := result.Append(listValueStarlarked); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrStarlarkConversion, err)
			}
		}

		return result, nil
	case []vfilter.Any:
		println("HEREINANY")
		result := starlark.NewList([]starlark.Value{})

		for _, item := range v {
			listValueStarlarked, err := interfaceAsStarlarkValue(item, ctx, scope)
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
			mapValueStarlarked, err := interfaceAsStarlarkValue(value, ctx, scope)
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
			mapValueStarlarked, err := interfaceAsStarlarkValue(value, ctx, scope)
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

		recurse_dict, err := reduceRecurse(v, ctx, scope)
		if err != nil {
			return nil, err
		}
		// Convert data to JSON and back to make sure it's types starlark can understand
		data, err := recurse_dict.(*ordereddict.Dict).MarshalJSON()
		if err != nil {
			return nil, err
		}
		new_dict := ordereddict.NewDict()
		err = new_dict.UnmarshalJSON(data)
		if err != nil {
			return nil, err
		}
		for key, value := range *new_dict.ToDict() {
			mapValueStarlarked, err := interfaceAsStarlarkValue(value, ctx, scope)
			if err != nil {
				return nil, err
			}

			result[key] = mapValueStarlarked
		}

		dict := starlarkstruct.FromStringDict(starlarkstruct.Default, result)

		return dict, nil

	case vfilter.LazyExpr:
		result, err := interfaceAsStarlarkValue(v.Reduce(ctx), ctx, scope)
		if err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%w: unsupported type %T", ErrStarlarkConversion, value)
	}
}

// recurse dicts and lazy expressions to make sure everything is reduced
func reduceRecurse(obj vfilter.Any, ctx context.Context, scope vfilter.Scope) (vfilter.Any, error) {
	switch t := obj.(type) {
	case *ordereddict.Dict:
		{
			sub_dict := ordereddict.NewDict()
			for key, value := range *t.ToDict() {
				results, err := reduceRecurse(value, ctx, scope)
				if err != nil {
					return nil, err
				}
				sub_dict.Set(key, results)
			}
			return sub_dict, nil
		}
	case vfilter.LazyExpr:
		{
			result, err := reduceRecurse(t.Reduce(ctx), ctx, scope)
			if err != nil {
				return nil, err
			}
			return result, nil
		}
	case *vfilter.LazyExprImpl:
		{
			result, err := reduceRecurse(t.Reduce(ctx), ctx, scope)
			if err != nil {
				return nil, err
			}
			return result, nil

		}
	case vfilter.StoredQuery:
		{
			row_channel := t.Eval(ctx, scope)
			rows := []*ordereddict.Dict{}
			for row := range row_channel {
				rows = append(rows, vfilter.RowToDict(ctx, scope, row))
			}
			return rows, nil
		}
	default:
		return obj, nil
	}
}

// Code modified from https://github.com/cirruslabs/cirrus-cli/blob/master/pkg/larker/converthook.go
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
		key = code
	}

	compiled_vars, ok := vql_subsystem.CacheGet(scope, key).(*ordereddict.Dict)
	if !ok {
		compiled_vars = ordereddict.NewDict()
		err := compileStarlark(code, globals, compiled_vars, ctx, scope)
		if err != nil {
			return nil, err
		}
		vql_subsystem.CacheSet(scope, key, compiled_vars)
	}

	return compiled_vars, nil
}

// User doesn't actually specify extras, that's done in extractStarlarkFuncArgs
type StarlarkFuncArgs struct {
	Func    string           `vfilter:"required,field=func,doc=Starlark function to call."`
	Code    string           `vfilter:"required,field=code,doc=Starlark code to compile."`
	Args    vfilter.LazyExpr `vfilter:"optional,field=args,doc=Positional args for the function."`
	Kwargs  vfilter.LazyExpr `vfilter:"optional,field=kwargs,doc=KW args for the function."`
	Key     string           `vfilter:"optional,field=key,doc=If set use this key to cache the Starlark code."`
	Extras  vfilter.Any      `vfilter:"optional,field=extras,doc="Extra Arguments to pass to Starlark code. Set implicitly"`
	Globals vfilter.LazyExpr `vfilter:"optional,field=globals,doc="Global Arguments to pass to Starlark code."`
}

type StarlarkFunc struct{}

func compileStarlark(code string, globals vfilter.Any, compiled_vars *ordereddict.Dict, ctx context.Context, scope vfilter.Scope) error {
	sthread := &starlark.Thread{Name: "VQL Thread", Load: starlib.Loader}
	starvals, err := interfaceAsStarlarkValue(globals, ctx, scope)
	if err != nil {
		return err

	}
	stringdict := starlark.StringDict{}
	switch t := starvals.(type) {
	case *starlarkstruct.Struct:
		t.ToStringDict(stringdict)
	case starlark.NoneType: // Do nothing
	default:
		return errors.New(fmt.Sprintf("Unrecognized data type %T provided to globals!", starvals))
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
		switch key {
		case "func", "code", "args", "kwargs", "key", "globals":
			result.Set(key, value)
		default:
			extra_args.Set(key, value)
		}
	}
	result.Set("extras", extra_args)
	return result
}

// Turn dicts into starlark tuples to pass to starlark.Call
func makeKwargsTuple(kwargs *ordereddict.Dict, ctx context.Context, scope vfilter.Scope) ([]starlark.Tuple, error) {
	starlark_args, err := interfaceAsStarlarkValue(kwargs, ctx, scope)
	if err != nil {
		return nil, err
	}
	starlark_tuple := make([]starlark.Tuple, 0)
	new_string_dict := starlark.StringDict{}
	switch t := starlark_args.(type) {
	case *starlarkstruct.Struct:
		t.ToStringDict(new_string_dict)
	default:
		return nil, errors.New(fmt.Sprintf("Unsupported Type %T", starlark_args))
	}
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
	sthread := &starlark.Thread{Name: "VQL Thread", Load: starlib.Loader}
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

		starlark_args, err := interfaceAsStarlarkValue(call_args, ctx, scope)
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
		switch t := arg.Kwargs.Reduce(ctx).(type) {
		case *ordereddict.Dict:
			{
				starlark_tuple, nil := makeKwargsTuple(t, ctx, scope)
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
			}
		default:
			scope.Log("starl_func: Non-dict type %T provided to kwargs!", arg.Kwargs)
			return vfilter.Null{}
		}

	} else if arg.Extras.(*ordereddict.Dict).Len() > 0 {
		starlark_tuple, nil := makeKwargsTuple(arg.Extras.(*ordereddict.Dict), ctx, scope)
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
	Code    string           `vfilter:"required,field=code,doc=Starlark code to compile."`
	Vars    []string         `vfilter:"optional,field=vars,doc=Name of variables to return. Returns all if not specified."`
	Key     string           `vfilter:"optional,field=key,doc=If set use this key to cache the Starlark code."`
	Globals vfilter.LazyExpr `vfilter:"optional,field=globals,doc="Global Arguments to pass to Starlark code."`
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
		for _, item := range arg.Vars {
			ret, ok := compiled_args.Get(item)
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
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	vql_subsystem.RegisterFunction(&StarlarkFunc{})
	vql_subsystem.RegisterFunction(&StarlarkVars{})
}
