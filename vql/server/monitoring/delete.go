package monitoring

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteEventsPluginArgs struct {
	Artifact string `vfilter:"required,field=artifact,doc=Name of artifact events to remove"`
	ClientId string `vfilter:"required,field=client_id,doc=Client ID of events to remove (use 'server' for server events)"`

	StartTime time.Time `vfilter:"optional,field=start_time,doc=Start time to be deleted"`
	EndTime   time.Time `vfilter:"optional,field=end_time,doc=End time to be deleted"`

	ReallyDoIt bool `vfilter:"optional,field=really_do_it,doc=If not specified, just show what files will be removed"`
}

type DeleteEventsPlugin struct{}

func (self DeleteEventsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "delete_events", args)()

		err := vql_subsystem.CheckAccess(scope, acls.DELETE_RESULTS)
		if err != nil {
			scope.Log("delete_events: %v", err)
			return
		}

		arg := &DeleteEventsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("delete_events: %v", err)
			return
		}

		if arg.StartTime.IsZero() {
			arg.StartTime = time.Unix(0, 0)
		}

		if arg.EndTime.IsZero() {
			arg.EndTime = time.Now()
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("delete_events: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("delete_events: Command can only run on the server")
			return
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("delete_events: %v", err)
			return
		}

		principal := vql_subsystem.GetPrincipal(scope)
		if principal == "" {
			scope.Log("delete_events: Username not specified")
			return
		}

		responses, err := launcher.DeleteEvents(ctx, config_obj,
			principal, arg.Artifact, arg.ClientId,
			arg.StartTime, arg.EndTime, services.DeleteFlowOptions{
				ReallyDoIt: arg.ReallyDoIt,
			})
		if err != nil {
			scope.Log("delete_events: %v", err)
			return
		}

		for _, resp := range responses {
			select {
			case <-ctx.Done():
				return
			case output_chan <- resp:
			}
		}
	}()

	return output_chan
}

func (self DeleteEventsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "delete_events",
		Doc:      "Delete all the files that make up a flow.",
		ArgType:  type_map.AddType(scope, &DeleteEventsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.DELETE_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DeleteEventsPlugin{})
}
