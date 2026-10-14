package unified_logs

import (
	"context"

	ulogs "github.com/Velocidex/go-macos-unifiedlogs"
	unifiedlogs "github.com/Velocidex/go-macos-unifiedlogs"
	"github.com/Velocidex/go-macos-unifiedlogs/filtering"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ParseUnifiedPluginArgs struct {
	BaseDir  *accessors.OSPath `vfilter:"required,field=base,doc=A base directory containing all the required files (.tracev3, .timesync, uuidtext, dsc)."`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Threads  int64             `vfilter:"optional,field=threads,doc=The number of threads to use (defalut 5)."`
}

type ParseUnifiedPlugin struct{}

func (self ParseUnifiedPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "parse_unified_logs", args)()
		defer utils.RecoverVQL(scope)

		arg := &ParseUnifiedPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_unified_logs: %s", err.Error())
			return
		}

		count := &utils.Counter{}

		scope.AddDestructor(func() {
			scope.Log("parse_unified_logs: Parsed %v events. Stats %#v",
				count.Get(), ulogs.GetStats())
		})

		provider, err := NewVeloFileProvider(ctx, scope, arg)
		if err != nil {
			scope.Log("parse_unified_logs: %s", err.Error())
			return
		}

		for _, l := range provider.Debug() {
			scope.Log("DEBUG: parse_unified_logs: %v", l)
		}

		ulogs.ResetStats()
		parser := ulogs.NewUnifiedLogParser(
			provider, provider.Logger(),
			unifiedlogs.NewMemoryStringCache(),
			ulogs.ParserOptions{
				Threads: int(arg.Threads),
			},
		)

		events, err := parser.Parse(ctx)
		if err != nil {
			scope.Log("parse_evtx: %s", err.Error())
			return
		}

		for l := range events {
			select {
			case <-ctx.Done():
				return
			case output_chan <- filtering.WrapLogData(&l):
				count.Inc()
			}
		}

	}()

	return output_chan
}

func (self ParseUnifiedPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_unified_logs",
		Doc:     "Parses MACOS unified logs.",
		ArgType: type_map.AddType(scope, &ParseUnifiedPluginArgs{}),
		Metadata: vql_subsystem.VQLMetadata().
			Permissions(acls.FILESYSTEM_READ).Build(),
		Version: 1,
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ParseUnifiedPlugin{})
}
