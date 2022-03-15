package tools

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type QueryPluginArgs struct {
	Query           vfilter.Any       `vfilter:"required,field=query,doc=A VQL Query to parse and execute."`
	Env             *ordereddict.Dict `vfilter:"optional,field=env,doc=A dict of args to insert into the scope."`
	CpuLimit        float64           `vfilter:"optional,field=cpu_limit,doc=Average CPU usage in percent of a core."`
	IopsLimit       float64           `vfilter:"optional,field=iops_limit,doc=Average IOPs to target."`
	ProgressTimeout float64           `vfilter:"optional,field=progress_timeout,doc=If no progress is detected in this many seconds, we terminate the query and output debugging information"`
}

type QueryPlugin struct{}

func (self QueryPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		// This plugin just passes the current scope to the
		// subquery so there is no permissions check - the
		// subquery will receive the same privileges as the
		// calling query.
		arg := &QueryPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("query: %v", err)
			return
		}

		// Build a completely new scope to evaluate the query
		// in.
		builder := services.ScopeBuilderFromScope(scope)

		// Make a new scope for each artifact.
		manager, err := services.GetRepositoryManager()
		if err != nil {
			scope.Log("query: %v", err)
			return
		}

		subscope := manager.BuildScope(builder).AppendVars(arg.Env)
		defer subscope.Close()

		throttler := actions.NewThrottler(
			ctx, subscope, 0, arg.CpuLimit, arg.IopsLimit)
		if arg.ProgressTimeout > 0 {
			subctx, cancel := context.WithCancel(ctx)
			ctx = subctx

			duration := time.Duration(arg.ProgressTimeout) * time.Second
			throttler = actions.NewProgressThrottler(
				subctx, scope, cancel, throttler, duration)
			scope.Log("query: Installing a progress alarm for %v", duration)
		}
		subscope.SetThrottler(throttler)

		runQuery(ctx, subscope, output_chan, arg.Query)
	}()

	return output_chan

}

func runQuery(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	query vfilter.Any) {

	switch t := query.(type) {
	case string:
		runStringQuery(ctx, scope, output_chan, t)

	case vfilter.StoredQuery:
		runStoredQuery(ctx, scope, output_chan, t)

	case vfilter.LazyExpr:
		runQuery(ctx, scope, output_chan, t.ReduceWithScope(ctx, scope))

	default:
		scope.Log("query: query should be a string or subquery")
		return
	}
}

func runStoredQuery(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	query vfilter.StoredQuery) {

	row_chan := query.Eval(ctx, scope)
	for {
		select {
		case <-ctx.Done():
			return

		case row, ok := <-row_chan:
			if !ok {
				return
			}
			output_chan <- row
		}
	}
}

func runStringQuery(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	query_string string) {

	// Parse and compile the query
	scope.Log("query: running query %v", query_string)
	statements, err := vfilter.MultiParse(query_string)
	if err != nil {
		scope.Log("query: %v", err)
		return
	}

	for _, vql := range statements {
		row_chan := vql.Eval(ctx, scope)
	get_rows:
		for {
			select {
			case <-ctx.Done():
				return

			case row, ok := <-row_chan:
				if !ok {
					break get_rows
				}

				output_chan <- row
			}
		}
	}
}

func (self QueryPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "query",
		Doc:     "Evaluate a VQL query.",
		ArgType: type_map.AddType(scope, &QueryPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&QueryPlugin{})
}
