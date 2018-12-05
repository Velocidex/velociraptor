package functions

import (
	"context"
	"fmt"
	"reflect"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type FormatArgs struct {
	Format string      `vfilter:"required,field=format"`
	Args   vfilter.Any `vfilter:"optional,field=args"`
}

type FormatFunction struct{}

func (self *FormatFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &FormatArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("format: %s", err.Error())
		return false
	}

	var format_args []interface{}
	slice := reflect.ValueOf(arg.Args)

	// A slice of strings.
	if slice.Type().Kind() != reflect.Slice {
		format_args = append(format_args, arg.Args)
	} else {
		for i := 0; i < slice.Len(); i++ {
			value := slice.Index(i).Interface()
			format_args = append(format_args, value)
		}
	}

	return fmt.Sprintf(arg.Format, format_args...)
}

func (self FormatFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "format",
		Doc:     "Format one or more items according to a format string.",
		ArgType: type_map.AddType(scope, &FormatArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&FormatFunction{})
}
