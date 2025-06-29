package tools

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/executor/throttler"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type QueryPluginArgs struct {
	Query           vfilter.Any       `vfilter:"required,field=query,doc=A VQL Query to parse and execute."`
	Env             *ordereddict.Dict `vfilter:"optional,field=env,doc=A dict of args to insert into the scope."`
	CopyEnv         []string          `vfilter:"optional,field=copy_env,doc=A list of variables in the current scope that will be copied into the new scope."`
	CpuLimit        float64           `vfilter:"optional,field=cpu_limit,doc=Average CPU usage in percent of a core."`
	IopsLimit       float64           `vfilter:"optional,field=iops_limit,doc=Average IOPs to target."`
	Timeout         float64           `vfilter:"optional,field=timeout,doc=Cancel the query after this many seconds"`
	ProgressTimeout float64           `vfilter:"optional,field=progress_timeout,doc=If no progress is detected in this many seconds, we terminate the query and output debugging information"`
	OrgId           string            `vfilter:"optional,field=org_id,doc=If specified, the query will run in the specified org space (Use 'root' to refer to the root org)"`
	Principal       string            `vfilter:"optional,field=runas,doc=If specified, the query will run as the specified user"`
	InheritScope    bool              `vfilter:"optional,field=inherit,doc=If specified we inherit the scope instead of building a new one."`
}

type QueryPlugin struct{}

func (self QueryPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "query", args)()

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

		// If we are not running on the server, we need to get the
		// root config from the org manager.
		org_manager, err := services.GetOrgManager()
		if err != nil {
			scope.Log("query: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			config_obj, err = org_manager.GetOrgConfig(services.ROOT_ORG_ID)
			if err != nil {
				scope.Log("query: %v", err)
				return
			}
		}
		org_config_obj := config_obj

		// Build a completely new scope to evaluate the query
		// in.
		builder := services.ScopeBuilderFromScope(scope)

		// Did the user request running in the specified org? Switch
		// orgs if so.
		if arg.OrgId != "" {
			if arg.InheritScope {
				scope.Log("query: inherit flag is specified at the same time as an org id - these options are not compatible!")
				return
			}

			org_config_obj, err = org_manager.GetOrgConfig(arg.OrgId)
			if err != nil {
				scope.Log("query: %v", err)
				return
			}

			// The subscoope will switch to the specified org.
			builder.Config = org_config_obj
		}

		if arg.Principal != "" {
			// Impersonation is only allowed for administrator users.
			err := vql_subsystem.CheckAccess(scope, acls.IMPERSONATION)
			if err != nil {
				scope.Log("ERROR:query: Permission required for runas: %v", err)
				return
			}

			// Run as the specified user.
			builder.ACLManager = acl_managers.NewServerACLManager(
				org_config_obj, arg.Principal)
		}

		// Make a new scope for each artifact.
		manager, err := services.GetRepositoryManager(org_config_obj)
		if err != nil {
			scope.Log("ERROR:query: %v", err)
			return
		}

		if arg.Timeout > 0 {
			subctx, cancel := context.WithTimeout(
				ctx, time.Duration(arg.Timeout)*time.Second)
			defer cancel()

			ctx = subctx
		}

		var subscope vfilter.Scope
		if arg.InheritScope {
			subscope = scope.Copy()

		} else {
			if arg.Env == nil {
				arg.Env = ordereddict.NewDict()
			}
			for _, env := range arg.CopyEnv {
				item, pres := scope.Resolve(env)
				if pres {
					arg.Env.Set(env, item)
				}
			}
			subscope = manager.BuildScope(builder).AppendVars(arg.Env)
		}
		defer subscope.Close()

		throttler, closer := throttler.NewThrottler(
			ctx, subscope, org_config_obj, 0, arg.CpuLimit, arg.IopsLimit)
		if arg.ProgressTimeout > 0 {
			subctx, cancel := context.WithCancel(ctx)
			ctx = subctx

			duration := time.Duration(arg.ProgressTimeout) * time.Second
			throttler = actions.NewProgressThrottler(
				subctx, scope, cancel, throttler, duration)
			scope.Log("query: Installing a progress alarm for %v", duration)
		}
		subscope.SetThrottler(throttler)
		err = subscope.AddDestructor(closer)
		if err != nil {
			scope.Log("query: %v", err)
		}

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
		scope.Log("ERROR:query: query should be a string or subquery")
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
		scope.Log("ERROR:query: %v", err)
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
		Name:     "query",
		Doc:      "Evaluate a VQL query.",
		ArgType:  type_map.AddType(scope, &QueryPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.IMPERSONATION).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&QueryPlugin{})
}
