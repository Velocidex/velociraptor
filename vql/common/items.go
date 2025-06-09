package common

import (
	"context"
	"reflect"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ItemsPluginArgs struct {
	Item vfilter.Any `vfilter:"optional,field=item,doc=The item to enumerate."`
}

type ItemsPlugin struct{}

func (self ItemsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "items", args)()

		arg := &ItemsPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("items: %v", err)
			return
		}

		switch t := arg.Item.(type) {

		case vfilter.StoredQuery:
			i := 0
			for row := range t.Eval(ctx, scope) {
				select {
				case <-ctx.Done():
					return

				case output_chan <- ordereddict.NewDict().
					Set("_key", i).
					Set("_value", row):
					i++
				}
			}
			return

		case vfilter.LazyExpr:
			arg.Item = t.Reduce(ctx)
		}

		a_value := reflect.Indirect(reflect.ValueOf(arg.Item))
		a_type := a_value.Type()

		if a_type.Kind() == reflect.Slice {
			for i := 0; i < a_value.Len(); i++ {
				element := a_value.Index(i).Interface()
				select {
				case <-ctx.Done():
					return

				case output_chan <- ordereddict.NewDict().
					Set("_key", i).
					Set("_value", element):
				}
			}
			return
		}

		members := scope.GetMembers(arg.Item)
		if len(members) > 0 {
			for _, key := range members {
				value, pres := scope.Associative(arg.Item, key)
				if pres {
					select {
					case <-ctx.Done():
						return

					case output_chan <- ordereddict.NewDict().
						Set("_key", key).
						Set("_value", value):
					}
				}
			}
		}

	}()

	return output_chan
}

func (self ItemsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "items",
		Doc:     "Enumerate all members of the item (similar to Pythons items() method.",
		ArgType: type_map.AddType(scope, &ItemsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ItemsPlugin{})
}
