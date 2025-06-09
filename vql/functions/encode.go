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
	defer vql_subsystem.RegisterMonitor(ctx, "encode", args)()

	arg := &EncodeFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("serialize: %s", err.Error())
		return vfilter.Null{}
	}

	result := arg.Item
	switch t := result.(type) {
	case types.LazyExpr:
		result = t.Reduce(ctx)

	case types.StoredQuery:
		result_rows := []vfilter.Row{}
		for row := range t.Eval(ctx, scope) {
			result_rows = append(result_rows, row)
		}

		result = result_rows
	}

	switch arg.Format {
	case "", "json":
		opts := vql_subsystem.EncOptsFromScope(scope)
		serialized_content, err := json.MarshalIndentWithOptions(result, opts)
		if err != nil {
			scope.Log("serialize: %v", err)
			return vfilter.Null{}
		}

		return string(serialized_content)

	case "yaml":
		serialized, err := yaml.Marshal(result)
		if err != nil {
			scope.Log("serialize: %v", err)
			return vfilter.Null{}
		}
		return string(serialized)

	case "hex":
		switch t := result.(type) {
		case []byte:
			return hex.EncodeToString(t)
		case string:
			return hex.EncodeToString([]byte(t))
		default:
			scope.Log("serialize: Unsupported type for hex encoding %T", result)
			return vfilter.Null{}
		}

	case "base64":
		switch t := result.(type) {
		case []byte:
			return base64.RawStdEncoding.EncodeToString(t)
		case string:
			return base64.RawStdEncoding.EncodeToString([]byte(t))
		default:
			scope.Log("serialize: Unsupported type for base64 encoding %T", result)
			return vfilter.Null{}
		}

	case "csv":
		// Not actually a slice.
		if reflect.TypeOf(result).Kind() != reflect.Slice {
			return vfilter.Null{}
		}

		config_obj, _ := vql_subsystem.GetServerConfig(scope)

		buff := bytes.NewBuffer([]byte{})
		csv_writer := csv.GetCSVAppender(config_obj,
			scope, buff,
			true, /* write_headers */
			json.DefaultEncOpts())

		result_rows_value := reflect.ValueOf(result)
		for i := 0; i < result_rows_value.Len(); i++ {
			csv_writer.Write(result_rows_value.Index(i).Interface())
		}
		csv_writer.Close()

		return buff.String()

	default:
		scope.Log("serialize: Unknown format %s", arg.Format)
	}
	return vfilter.Null{}
}

func (self EncodeFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "serialize",
		Doc:     "Encode an object as a string (csv or json).",
		ArgType: type_map.AddType(scope, &EncodeFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&EncodeFunction{})
}
