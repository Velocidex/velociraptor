package functions

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"

	"www.velocidex.com/golang/velociraptor/file_store/csv"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type EncodeFunctionArgs struct {
	Item   vfilter.Any `vfilter:"required,field=item,doc=The item to encode"`
	Format string      `vfilter:"optional,field=format,doc=Encoding format (csv,json)"`
}

type EncodeFunction struct{}

func (self *EncodeFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &EncodeFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("serialize: %s", err.Error())
		return vfilter.Null{}
	}

	result := arg.Item
	switch t := result.(type) {
	case vfilter.LazyExpr:
		result = t.Reduce()

	case vfilter.StoredQuery:
		result_rows := []vfilter.Row{}
		for row := range t.Eval(ctx, scope) {
			result_rows = append(result_rows, row)
		}

		result = result_rows
	}

	switch arg.Format {
	case "", "json":
		serialized_content, err := json.MarshalIndent(result, "", "")
		if err != nil {
			scope.Log("serialize: %s", err.Error())
			return vfilter.Null{}
		}

		return serialized_content

	case "csv":
		// Not actually a slice.
		if reflect.TypeOf(result).Kind() != reflect.Slice {
			return vfilter.Null{}
		}
		buff := bytes.NewBuffer([]byte{})
		csv_writer := csv.GetCSVAppender(
			scope, buff,
			true /* write_headers */)

		result_rows_value := reflect.ValueOf(result)
		for i := 0; i < result_rows_value.Len(); i++ {
			csv_writer.Write(result_rows_value.Index(i).Interface())
		}
		csv_writer.Close()

		return buff.String()

	default:
		scope.Log("encode: Unknown format %s", arg.Format)
	}
	return vfilter.Null{}
}

func (self EncodeFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "serialize",
		Doc:     "Encode an object as a string (csv or json).",
		ArgType: type_map.AddType(scope, &EncodeFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&EncodeFunction{})
}
