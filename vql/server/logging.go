package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	AllComponents = []string{
		"client", "frontend", "gui", "api", "tool", "generic"}
)

type logRow struct {
	Time  time.Time `json:"time"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
}

type LogsPluginArgs struct {
	Components    []string `vfilter:"optional,field=component,docs=Requested component (default all components)"`
	IncludePrelog bool     `vfilter:"optional,field=prelog,docs=Include all logs up to this point"`
}

type LogsPlugin struct{}

func (self LogsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "logging", args)()

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
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

		if len(arg.Components) == 0 {
			arg.Components = AllComponents
		}

		if arg.IncludePrelog {
			for _, line := range logging.GetMemoryLogs() {
				record := &logRow{}
				err := json.Unmarshal([]byte(line), record)
				if err != nil {
					continue
				}

				record.Time = record.Time.UTC()
				record.Level = strings.ToUpper(record.Level)

				select {
				case <-ctx.Done():
					return
				case output_chan <- record:
				}
			}
		}

		wg := &sync.WaitGroup{}
		defer wg.Wait()

		for _, c := range arg.Components {
			wg.Add(1)
			go func(c string) {
				defer wg.Done()

				var target *string

				switch c {
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
				case "generic":
					target = &logging.GenericComponent

				default:
					scope.Log("logging: Unknown component %v: The following are supported %v",
						c, AllComponents)
					return
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

						record := &logRow{}
						err := json.Unmarshal([]byte(line), record)
						if err != nil {
							continue
						}

						record.Time = record.Time.UTC()
						record.Level = strings.ToUpper(record.Level)

						select {
						case <-ctx.Done():
							return
						case output_chan <- record:
						}
					}
				}
			}(c)
		}
	}()

	return output_chan
}

func (self LogsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "logging",
		Doc:     "Watch the logs emitted by the server.",
		ArgType: type_map.AddType(scope, &LogsPluginArgs{}),

		// Requires SERVER_ADMIN because this provides access to all
		// server logs
		Metadata: vql.VQLMetadata().Permissions(acls.SERVER_ADMIN).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&LogsPlugin{})
}
