package golang

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type BackgroundArgs struct {
	Query types.StoredQuery `vfilter:"optional,field=query,doc=Run this query in the background."`
}

type BackgroundFunction struct{}

func (self *BackgroundFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "background", args)()

	arg := &BackgroundArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("background: %s", err.Error())
		return false
	}

	// Cancel the subquery when the scope closes.
	sub_ctx, cancel := context.WithCancel(ctx)
	err = scope.AddDestructor(cancel)
	if err != nil {
		cancel()
		scope.Log("background: %s", err.Error())
		return false
	}

	go func() {
		for item := range arg.Query.Eval(sub_ctx, scope) {
			// We still materialize the entire row but we discard the
			// results.
			_ = vfilter.MaterializedLazyRow(ctx, item, scope)
			select {
			case <-ctx.Done():
				return
			}
		}
	}()

	return true
}

func (self BackgroundFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "background",
		Doc:     "Run a query in the background. All output from the query is discarded",
		ArgType: type_map.AddType(scope, &BackgroundArgs{}),
		Version: 1,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&BackgroundFunction{})
}
