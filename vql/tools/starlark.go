package tools

import (
	"context"
	"errors"

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

var ErrStarlarkConversion = errors.New("failed to convert Starlark data type")

// Convert starlark types to Golang and VQL types
// Code modified from https://github.com/cirruslabs/cirrus-cli/blob/master/pkg/larker/converthook.go
func starlarkValueAsInterface(value starlark.Value) (interface{}, error) {
	switch v := value.(type) {
	case *starlark.Function:
		return &starlarkFuncWrapper{delegate: v}, nil

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
			key := item[0].String()
			value := item[1]

			dictValueInterfaced, err := starlarkValueAsInterface(value)
			if err != nil {
				return nil, err
			}
			result.Set(key[1:len(key)-1], dictValueInterfaced)
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
		return result, nil
	default:
		return nil, fmt.Errorf("%w: unsupported type %T", ErrStarlarkConversion, value)
	}
}

// Convert Golang and VQL types to Starlark Types
// Code modified from https://github.com/cirruslabs/cirrus-cli/blob/master/pkg/larker/converthook.go
func interfaceAsStarlarkValue(ctx context.Context,
	scope vfilter.Scope, value interface{}) (starlark.Value, error) {
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
			listValueStarlarked, err := interfaceAsStarlarkValue(ctx, scope, item)
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
			listValueStarlarked, err := interfaceAsStarlarkValue(ctx, scope, item)
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
			mapValueStarlarked, err := interfaceAsStarlarkValue(ctx, scope, value)
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
			mapValueStarlarked, err := interfaceAsStarlarkValue(ctx, scope, value)
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
			mapValueStarlarked, err := interfaceAsStarlarkValue(ctx, scope, value)
			if err != nil {
				return nil, err
			}

			result[key] = mapValueStarlarked
		}

		dict := starlarkstruct.FromStringDict(starlarkstruct.Default, result)

		return dict, nil

	case vfilter.LazyExpr:
		result, err := interfaceAsStarlarkValue(ctx, scope, v.Reduce(ctx))
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

func compileStarlark(ctx context.Context, scope types.Scope,
	code string, globals vfilter.Any) (*ordereddict.Dict, error) {

	sthread := &starlark.Thread{Name: "VQL Thread", Load: starlib.Loader}
	starvals, err := interfaceAsStarlarkValue(ctx, scope, globals)
	if err != nil {
		return nil, err

	}
	stringdict := starlark.StringDict{}
	switch t := starvals.(type) {

	case *starlarkstruct.Struct:
		t.ToStringDict(stringdict)

	case starlark.NoneType: // Do nothing

	default:
		return nil, fmt.Errorf("Unrecognized data type %T provided to globals!", starvals)
	}

	new_globals, err := starlark.ExecFile(sthread, "apparent/filename.star", code, stringdict)
	if err != nil {
		return nil, err
	}

	compiled_vars := ordereddict.NewDict()
	for key, item := range new_globals {
		entry, err := starlarkValueAsInterface(item)
		if err != nil {
			return nil, err
		}
		compiled_vars.Set(key, entry)
	}

	return compiled_vars, nil
}

// Turn dicts into starlark tuples to pass to starlark.Call
func makeKwargsTuple(ctx context.Context, scope vfilter.Scope,
	kwargs *ordereddict.Dict) ([]starlark.Tuple, error) {
	starlark_args, err := interfaceAsStarlarkValue(ctx, scope, kwargs)
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

type StarlarkCompileArgs struct {
	Code    string      `vfilter:"required,field=code,doc=The body of the starlark code."`
	Key     string      `vfilter:"optional,field=key,doc=If set use this key to cache the Starlark code block."`
	Globals vfilter.Any `vfilter:"optional,field=globals,doc=Dictionary of values to feed into Starlark environment"`
}

type StarlarkCompileFunction struct{}

func (self StarlarkCompileFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := StarlarkCompileArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, &arg)
	if err != nil {
		scope.Log("starl: %s", err.Error())
		return vfilter.Null{}
	}

	defer logIfPanic(scope)

	// grab compiled code
	compiled_args, err := compileStarlark(ctx, scope, arg.Code, arg.Globals)
	if err != nil {
		scope.Log("starl: %v", err)
		return vfilter.Null{}
	}

	return compiled_args
}

func (self StarlarkCompileFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "starl",
		Doc:     "Compile a starlark code block - returns a module usable in VQL",
		ArgType: type_map.AddType(scope, &StarlarkCompileArgs{}),
	}
}

type starlarkFuncWrapper struct {
	delegate *starlark.Function
}

func (self starlarkFuncWrapper) Copy() types.FunctionInterface {
	return self
}

func (self starlarkFuncWrapper) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) types.Any {

	// create new thread per call
	sthread := &starlark.Thread{Name: "VQL Thread", Load: starlib.Loader}
	kwargs, err := makeKwargsTuple(ctx, scope, args)
	if err != nil {
		scope.Log("starl: %v", err)
		return vfilter.Null{}
	}

	value, err := starlark.Call(sthread, self.delegate, starlark.Tuple{}, kwargs)
	if err != nil {
		scope.Log("starl: %s", err.Error())
		return vfilter.Null{}
	}

	result, err := starlarkValueAsInterface(value)
	if err != nil {
		scope.Log("starl: %v", err)
		return vfilter.Null{}
	}
	return result
}

func (self starlarkFuncWrapper) Info(scope types.Scope,
	type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{}
}

func init() {
	// Must be set to allow recursion and starlark sets
	resolve.AllowSet = true
	resolve.AllowRecursion = true
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	vql_subsystem.RegisterFunction(&StarlarkCompileFunction{})
}
