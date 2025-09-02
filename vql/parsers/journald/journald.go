package journald

import (
	"context"
	"time"

	"github.com/Velocidex/go-journalctl/parser"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type JournalPluginArgs struct {
	Filenames []*accessors.OSPath `vfilter:"required,field=filename,doc=A list of journal log files to parse."`
	Accessor  string              `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Raw       bool                `vfilter:"optional,field=raw,doc=Emit raw events (not parsed)."`
	StartTime time.Time           `vfilter:"optional,field=start_time,doc=Only parse events newer than this time (default all times)."`
	EndTime   time.Time           `vfilter:"optional,field=end_time,doc=Only parse events older than this time (default all times)."`
}

type JournalPlugin struct{}

func (self JournalPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parse_journald",
		Doc:      "Parse a journald file.",
		ArgType:  type_map.AddType(scope, &JournalPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func (self JournalPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "parse_journald", args)()

		arg := &JournalPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parse_journald: %v", err)
			return
		}

		for _, filename := range arg.Filenames {
			func() {
				// Choose a managed reader because we will return raw
				// objects.
				reader, err := readers.NewAccessorReader(
					scope, arg.Accessor, filename, 1000)
				if err != nil {
					scope.Log("parse_journald: %v", err)
					return
				}
				defer reader.Close()

				journal, err := parser.OpenFile(reader)
				if err != nil {
					scope.Log("parse_journald: %v", err)
					return
				}

				journal.RawLogs = arg.Raw
				journal.MinTime = arg.StartTime
				journal.MaxTime = arg.EndTime

				for log := range journal.GetLogs(ctx) {
					select {
					case <-ctx.Done():
						return
					case output_chan <- log:
					}
				}
			}()
		}
	}()

	return output_chan
}

type WatchJournalPluginArgs struct {
	Filenames []*accessors.OSPath `vfilter:"required,field=filename,doc=A list of journal log files to parse."`
	Accessor  string              `vfilter:"optional,field=accessor,doc=The accessor to use."`
	Raw       bool                `vfilter:"optional,field=raw,doc=Emit raw events (not parsed)."`
}

type WatchJournaldPlugin struct{}

func (self WatchJournaldPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "watch_journald", args)()

		arg := &WatchJournalPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_journald: %v", err)
			return
		}

		event_channel := make(chan vfilter.Row)

		// Register the output channel as a listener to the
		// global event.
		for _, filename := range arg.Filenames {
			cancel := gJournaldService.Register(
				filename, arg.Accessor, ctx, scope,
				arg.Raw, event_channel)

			defer cancel()
		}

		// Wait until the query is complete.
		for {
			select {
			case <-ctx.Done():
				return

			case event := <-event_channel:
				select {
				case <-ctx.Done():
					return

				case output_chan <- event:
				}
			}
		}
	}()

	return output_chan
}

func (self WatchJournaldPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "watch_journald",
		Doc:      "Watch a journald file and stream events from it. ",
		ArgType:  type_map.AddType(scope, &WatchJournalPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&JournalPlugin{})
	vql_subsystem.RegisterPlugin(&WatchJournaldPlugin{})
}
