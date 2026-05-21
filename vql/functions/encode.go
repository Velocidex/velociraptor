package functions

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"reflect"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type EncodeFunctionArgs struct {
	Item   vfilter.Any `vfilter:"required,field=item,doc=The item to encode"`
	Format string      `vfilter:"optional,field=format,doc=Encoding format (csv,json,yaml,hex,base64)"`
}

type EncodeFunction struct{}

func (self *EncodeFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "serialize", args)()

	arg := &EncodeFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("serialize: %s", err.Error())
		return vfilter.Null{}
	}

	return _encode("serialize", ctx, scope, arg.Item, arg.Format)
}

func _encode(
	name string,
	ctx context.Context, scope vfilter.Scope,
	item vfilter.Any, format string) vfilter.Any {

	switch t := item.(type) {
	case types.LazyExpr:
		item = t.Reduce(ctx)

	case types.StoredQuery:
		result_rows := []vfilter.Row{}
		for row := range t.Eval(ctx, scope) {
			result_rows = append(result_rows, row)
		}

		item = result_rows
	}

	switch format {
	case "", "json":
		opts := vql_subsystem.EncOptsFromScope(scope)
		serialized_content, err := json.MarshalIndentWithOptions(item, opts)
		if err != nil {
			scope.Log("%s: %v", name, err)
			return vfilter.Null{}
		}

		return string(serialized_content)

	case "yaml":
		serialized, err := yaml.Marshal(item)
		if err != nil {
			scope.Log("%v: %v", name, err)
			return vfilter.Null{}
		}
		return string(serialized)

	case "hex":
		switch t := item.(type) {
		case []byte:
			return hex.EncodeToString(t)
		case string:
			return hex.EncodeToString([]byte(t))
		default:
			scope.Log("%s: Unsupported type for hex encoding %T", name, item)
			return vfilter.Null{}
		}

	case "base64":
		switch t := item.(type) {
		case []byte:
			return base64.RawStdEncoding.EncodeToString(t)
		case string:
			return base64.RawStdEncoding.EncodeToString([]byte(t))
		default:
			scope.Log("%v: Unsupported type for base64 encoding %T",
				name, item)
			return vfilter.Null{}
		}

	case "csv":
		// Not actually a slice.
		if reflect.TypeOf(item).Kind() != reflect.Slice {
			return vfilter.Null{}
		}

		config_obj, _ := vql_subsystem.GetServerConfig(scope)

		buff := bytes.NewBuffer([]byte{})
		csv_writer := csv.GetCSVAppender(config_obj,
			scope, buff,
			true, /* write_headers */
			json.DefaultEncOpts())

		result_rows_value := reflect.ValueOf(item)
		for i := 0; i < result_rows_value.Len(); i++ {
			csv_writer.Write(result_rows_value.Index(i).Interface())
		}
		csv_writer.Close()

		return buff.String()

	default:
		scope.Log("%s: Unknown format %s", name, format)
	}
	return vfilter.Null{}
}

func (self EncodeFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "serialize",
		Doc:     "Encode an object as a string (json, yaml, hex, base64, csv). This function is an alias to `encode`",
		ArgType: type_map.AddType(scope, &EncodeFunctionArgs{}),
	}
}

type _EncodeOverrideArgs struct {
	String types.Any `vfilter:"required,field=string,doc=The item to encode"`
	Type   string    `vfilter:"required,field=type,doc=Encoding format (csv,json,yaml,hex,base64)"`
}

type EncodeOverride struct{}

func (self EncodeOverride) Call(
	ctx context.Context,
	scope types.Scope,
	args *ordereddict.Dict) types.Any {
	arg := &_EncodeOverrideArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("encode: %s", err.Error())
		return types.Null{}
	}

	return _encode("encode", ctx, scope, arg.String, arg.Type)
}

func (self EncodeOverride) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "encode",
		Doc:     "Encode an object as a string (json, yaml, hex, base64, csv).",
		ArgType: type_map.AddType(scope, &_EncodeOverrideArgs{}),
		Version: 2,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&EncodeFunction{})
	vql_subsystem.OverrideFunction(&EncodeOverride{})
}
