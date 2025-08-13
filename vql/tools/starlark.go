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
	"www.velocidex.com/golang/velociraptor/json"
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
			key := item[0]
			value := item[1]

			dictValueInterfaced, err := starlarkValueAsInterface(value)
			if err != nil {
				return nil, err
			}

			dictKeyInterfaced, err := starlarkValueAsInterface(key)
			if err != nil {
				return nil, err
			}

			// JSON keys must be strings so we need to
			// convert from the Starlark type to a string.
			switch t := dictKeyInterfaced.(type) {
			case string:
				result.Set(t, dictValueInterfaced)

			case fmt.Stringer:
				result.Set(t.String(), dictValueInterfaced)

			case float64:
				result.Set(fmt.Sprintf("%f", t),
					dictValueInterfaced)

			case int64:
				result.Set(fmt.Sprintf("%d", t),
					dictValueInterfaced)
			}
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

		result := starlark.NewDict(new_dict.Len())
		for key, value := range new_dict.ToMap() {
			mapValueStarlarked, err := interfaceAsStarlarkValue(ctx, scope, value)
			if err != nil {
				return nil, err
			}

			err = result.SetKey(starlark.String(key), mapValueStarlarked)
			if err != nil {
				return nil, err
			}
		}

		//dict := starlarkstruct.FromStringDict(starlarkstruct.Default, result)

		return result, nil

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
			for key, value := range t.ToMap() {
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
	code string, globals vfilter.Any) (*StarlModule, error) {

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

	return &StarlModule{
		Dict:    compiled_vars,
		Code:    code,
		Globals: globals,
	}, nil
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

	case *starlark.Dict:
		return t.Items(), nil

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

	defer vql_subsystem.RegisterMonitor(ctx, "starl", args)()

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

	// Cancel the thread when we are done.
	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-sub_ctx.Done()
		sthread.Cancel("Cancelled")
	}()

	kwargs, err := makeKwargsTuple(ctx, scope, args)
	if err != nil {
		scope.Log("starl: %v", err)
		return vfilter.Null{}
	}

	value, err := starlark.Call(sthread, self.delegate, starlark.Tuple{}, kwargs)
	if err != nil {
		scope.Log("starl: %v", err)
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

// A thin wrapper that will be stored in the VQL scope. Supports
// marshalling.
type StarlModule struct {
	*ordereddict.Dict

	Code    string
	Globals interface{}
}

type starlModuleSerialized struct {
	Code string
}

// The types.Marshaler interface
func (self *StarlModule) Marshal(scope types.Scope) (*types.MarshalItem, error) {

	serialized, err := json.Marshal(starlModuleSerialized{
		Code: self.Code,
	})
	return &types.MarshalItem{
		Type: "StarlModule",
		Data: serialized,
	}, err
}

func (self StarlModule) Unmarshal(unmarshaller types.Unmarshaller,
	scope types.Scope, item *types.MarshalItem) (interface{}, error) {
	startl_module := &starlModuleSerialized{}
	err := json.Unmarshal(item.Data, &startl_module)
	if err != nil {
		return nil, err
	}

	return compileStarlark(context.Background(), scope,
		startl_module.Code, nil)
}

func init() {
	// Must be set to allow recursion and starlark sets
	resolve.AllowSet = true
	resolve.AllowRecursion = true
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowGlobalReassign = true
	vql_subsystem.RegisterFunction(&StarlarkCompileFunction{})
	vql_subsystem.RegisterProtocol(&StarlModuleAssociative{})
}

// Define some protocols.

// Modules are associative
type StarlModuleAssociative struct{}

func (self StarlModuleAssociative) Applicable(a types.Any, b types.Any) bool {
	_, a_ok := a.(*StarlModule)
	_, b_ok := b.(string)
	return a_ok && b_ok
}

func (self StarlModuleAssociative) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {
	a_value, a_ok := a.(*StarlModule)
	if a_ok {
		return scope.GetMembers(a_value.Dict)
	}
	return nil
}

func (self StarlModuleAssociative) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {
	a_value, a_ok := a.(*StarlModule)
	if a_ok {
		return scope.Associative(a_value.Dict, b)
	}
	return vfilter.Null{}, false
}
