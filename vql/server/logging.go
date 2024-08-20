package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type LogsPluginArgs struct {
	Component string `vfilter:"optional,field=component,docs=Requested component (default frontend)"`
}

type LogsPlugin struct{}

func (self LogsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor("logging", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("logging: %s", err)
			return
		}

		arg := &LogsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("logging: %v", err)
			return
		}

		var target *string

		switch arg.Component {
		case "client":
			target = &logging.ClientComponent
		case "frontend":
			target = &logging.FrontendComponent
		case "gui":
			target = &logging.GUIComponent
		case "api":
			target = &logging.APICmponent
		case "tool":
			target = &logging.ToolComponent
		default:
			target = &logging.GenericComponent
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			config_obj = &config_proto.Config{}
		}

		in := make(chan string)
		logger := logging.GetLogger(config_obj, target)
		closer := logger.AddListener(in)
		defer closer()

		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-in:
				if !ok {
					return
				}
				output_chan <- ordereddict.NewDict().
					Set("Log", line)
			}
		}
	}()

	return output_chan
}

func (self LogsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "logging",
		Doc:      "Watch the logs emitted by the server.",
		ArgType:  type_map.AddType(scope, &LogsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&LogsPlugin{})
}
