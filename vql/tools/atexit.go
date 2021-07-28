package tools

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type AtExitFunctionArgs struct {
	Query   types.Any         `vfilter:"required,field=query,doc=A VQL Query to parse and execute."`
	Env     *ordereddict.Dict `vfilter:"optional,field=env,doc=A dict of args to insert into the scope."`
	Timeout uint64            `vfilter:"optional,field=timeout,doc=How long to wait for destructors to run (default 60 seconds)."`
}

type AtExitFunction struct{}

func (self AtExitFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &AtExitFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("atexit: %v", err)
		return vfilter.Null{}
	}

	timeout := arg.Timeout
	if timeout == 0 {
		timeout = 60
	}

	switch t := arg.Query.(type) {
	case types.StoredQuery:
		scope.AddDestructor(func() {
			scope.Log("Running AtExit query")

			// We need to create a new context to run the
			// desctructors in because the main context
			// may already be cancelled.
			ctx, cancel := context.WithTimeout(
				context.Background(),
				time.Duration(timeout)*time.Second)
			defer cancel()

			subscope := scope.Copy()
			subscope.AppendVars(arg.Env)

			for _ = range t.Eval(ctx, subscope) {
			}
		})
	default:
		scope.Log("atexit: Query type %T not supported.", arg.Query)
	}

	return vfilter.Null{}
}

func (self AtExitFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "atexit",
		Doc:     "Install a query to run when the query is unwound.",
		ArgType: type_map.AddType(scope, &AtExitFunctionArgs{}),
		Version: 1,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AtExitFunction{})
}
