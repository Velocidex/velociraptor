package tools

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type QueryPluginArgs struct {
	Query string    `vfilter:"required,field=query,doc=A VQL Query to parse and execute."`
	Env   types.Any `vfilter:"optional,field=env,doc=A dict of args to insert into the scope."`
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

		arg_value, ok := arg.Env.(types.LazyExpr)
		if ok {
			arg.Env = arg_value.Reduce(ctx)
		}

		// Build the query args
		env := ordereddict.NewDict()
		for _, member := range scope.GetMembers(arg.Env) {
			v, _ := scope.Associative(arg.Env, member)
			env.Set(member, v)
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

		subscope := manager.BuildScope(builder).AppendVars(env)
		defer subscope.Close()

		// Parse and compile the query
		scope.Log("query: running query %v", arg.Query)
		statements, err := vfilter.MultiParse(arg.Query)
		if err != nil {
			scope.Log("query: %v", err)
			return
		}

		for _, vql := range statements {
			row_chan := vql.Eval(ctx, subscope)
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
	}()

	return output_chan
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
