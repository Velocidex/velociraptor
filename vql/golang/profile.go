package golang

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/tink-ab/tempfile"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/tools/process"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ProfilePluginArgs struct {
	Allocs    bool  `vfilter:"optional,field=allocs,doc=A sampling of all past memory allocations"`
	Block     bool  `vfilter:"optional,field=block,doc=Stack traces that led to blocking on synchronization primitives"`
	Goroutine bool  `vfilter:"optional,field=goroutine,doc=Stack traces of all current goroutines"`
	Heap      bool  `vfilter:"optional,field=heap,doc=A sampling of memory allocations of live objects."`
	Mutex     bool  `vfilter:"optional,field=mutex,doc=Stack traces of holders of contended mutexes"`
	Profile   bool  `vfilter:"optional,field=profile,doc=CPU profile."`
	Trace     bool  `vfilter:"optional,field=trace,doc=CPU trace."`
	Debug     int64 `vfilter:"optional,field=debug,doc=Debug level"`
	Logs      bool  `vfilter:"optional,field=logs,doc=Recent logs"`
	Queries   bool  `vfilter:"optional,field=queries,doc=Recent Queries run"`
	Metrics   bool  `vfilter:"optional,field=metrics,doc=Collect metrics"`
	Duration  int64 `vfilter:"optional,field=duration,doc=Duration of samples (default 30 sec)"`
}

func remove(scope vfilter.Scope, name string) {
	scope.Log("profile: removing tempfile %v", name)

	// On windows especially we can not remove files that
	// are opened by something else, so we keep trying for
	// a while.
	for i := 0; i < 100; i++ {
		err := os.Remove(name)
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
}

func writeMetrics(
	ctx context.Context, scope vfilter.Scope, output_chan chan vfilter.Row) {
	metrics, err := getMetrics()
	if err != nil {
		scope.Log("profile: while gathering metrics: %v", err)
		return
	}

	for _, metric := range metrics {
		select {
		case <-ctx.Done():
			return
		case output_chan <- ordereddict.NewDict().
			Set("Type", "metrics").
			Set("Line", metric).
			Set("FullPath", ""):
		}
	}
}

func writeProfile(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row, name string, debug int64) {
	tmpfile, err := ioutil.TempFile("", "tmp*.tmp")
	if err != nil {
		scope.Log("profile: %s", err)
		return
	}
	defer tmpfile.Close()

	dest := func() { remove(scope, tmpfile.Name()) }
	err = scope.AddDestructor(dest)
	if err != nil {
		dest()
		scope.Log("profile: %s", err)
		return
	}

	p := pprof.Lookup(name)
	if p == nil {
		scope.Log("profile: profile type %s not known", name)
		return
	}

	err = p.WriteTo(tmpfile, int(debug))
	if err != nil {
		scope.Log("profile: %s", err)
		return
	}

	select {
	case <-ctx.Done():
		return
	case output_chan <- ordereddict.NewDict().
		Set("Type", name).
		Set("Line", fmt.Sprintf("Generating profile %v", name)).
		Set("FullPath", tmpfile.Name()):
	}
}

func writeCPUProfile(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row, duration int64) {
	tmpfile, err := tempfile.TempFile("", "tmp", ".tmp")
	if err != nil {
		scope.Log("profile: %s", err)
		return
	}
	defer tmpfile.Close()

	err = scope.AddDestructor(func() { remove(scope, tmpfile.Name()) })
	if err != nil {
		scope.Log("profile: %s", err)
		return
	}

	err = pprof.StartCPUProfile(tmpfile)
	if err != nil {
		scope.Log("profile: %s", err)
		return
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(duration) * time.Second):
	}
	pprof.StopCPUProfile()

	select {
	case <-ctx.Done():
		return
	case output_chan <- ordereddict.NewDict().
		Set("Type", "profile").
		Set("Line", "Generating CPU profile").
		Set("FullPath", tmpfile.Name()):
	}
}

func writeTraceProfile(
	ctx context.Context,
	scope vfilter.Scope,
	output_chan chan vfilter.Row, duration int64) {
	tmpfile, err := tempfile.TempFile("", "tmp", ".tmp")
	if err != nil {
		scope.Log("profile: %s", err)
		return
	}
	defer tmpfile.Close()

	scope.AddDestructor(func() { remove(scope, tmpfile.Name()) })

	err = trace.Start(tmpfile)
	if err != nil {
		scope.Log("profile: %s", err)
		return
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(duration) * time.Second):
	}
	trace.Stop()

	select {
	case <-ctx.Done():
		return
	case output_chan <- ordereddict.NewDict().
		Set("Type", "trace").
		Set("Line", "Generating Trace profile").
		Set("FullPath", tmpfile.Name()):
	}
}

type ProfilePlugin struct{}

func (self *ProfilePlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("profile: %s", err)
			return
		}

		arg := &ProfilePluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("profile: %s", err.Error())
			return
		}

		if arg.Duration == 0 {
			arg.Duration = 30
		}

		if arg.Allocs {
			writeProfile(ctx, scope, output_chan, "allocs", arg.Debug)
		}

		if arg.Block {
			writeProfile(ctx, scope, output_chan, "block", arg.Debug)
		}

		if arg.Goroutine {
			writeProfile(ctx, scope, output_chan, "goroutine", arg.Debug)
		}

		if arg.Heap {
			writeProfile(ctx, scope, output_chan, "heap", arg.Debug)
		}

		if arg.Mutex {
			writeProfile(ctx, scope, output_chan, "mutex", arg.Debug)
		}

		if arg.Profile {
			writeCPUProfile(ctx, scope, output_chan, arg.Duration)
		}

		if arg.Trace {
			writeTraceProfile(ctx, scope, output_chan, arg.Duration)
		}

		if arg.Metrics {
			writeMetrics(ctx, scope, output_chan)
			select {
			case <-ctx.Done():
				return

			case output_chan <- ordereddict.NewDict().
				Set("Type", "process_tracker").
				Set("Line", process.GetGlobalTracker().Stats()):
			}
		}

		if arg.Logs {
			for _, line := range logging.GetMemoryLogs() {
				select {
				case <-ctx.Done():
					return

				case output_chan <- ordereddict.NewDict().
					Set("Type", "logs").
					Set("Line", line).
					Set("FullPath", ""):
				}
			}
		}

		if arg.Queries {
			for _, q := range actions.QueryLog.Get() {
				select {
				case <-ctx.Done():
					return

				case output_chan <- ordereddict.NewDict().
					Set("Type", "query").
					Set("Line", q).
					Set("FullPath", ""):
				}
			}
		}

	}()

	return output_chan
}

func (self ProfilePlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "profile",
		Doc:      "Returns a profile dump from the running process.",
		ArgType:  type_map.AddType(scope, &ProfilePluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ProfilePlugin{})
}
