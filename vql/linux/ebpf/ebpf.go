//go:build linux && (arm64 || amd64)
// +build linux
// +build arm64 amd64

package ebpf

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/tracee_velociraptor/manager"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	mu              sync.Mutex
	gEbpfManager    *manager.EBPFManager
	gEbpfIncludeEnv bool
)

type EBPFEventPluginArgs struct {
	EventNames     []string `vfilter:"optional,field=events,doc=A list of event names to acquire."`
	IncludeEnv     bool     `vfilter:"optional,field=include_env,doc=Include process environment variables."`
	Policy         string   `vfilter:"optional,field=policy,doc=Use a tracee policy in YAML format to specify events instead."`
	RegexPrefilter string   `vfilter:"optional,field=regex_prefilter,doc=A regex that must match the raw buffer before we process it."`
}

type EBPFEventPlugin struct{}

func (self EBPFEventPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "watch_ebpf",
		Doc:     "Watch for events from eBPF.",
		ArgType: type_map.AddType(scope, &EBPFEventPluginArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(
			acls.MACHINE_STATE).Build(),
		Version: 2,
	}
}

func (self EBPFEventPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "watch_ebpf", args)()
		defer utils.RecoverVQL(scope)

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("watch_ebpf: %s", err)
			return
		}

		arg := &EBPFEventPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("watch_ebpf: %s", err.Error())
			return
		}

		if arg.Policy == "" {
			if len(arg.EventNames) == 0 {
				scope.Log("watch_ebpf: should provide a policy or a list of events")
				return
			}
			arg.Policy = generateDefaultPolicy(arg.EventNames)
		}

		ebpf_manager, err := getEbpfManager(scope, arg)
		if err != nil {
			scope.Log("watch_ebpf: %v", err)
			return
		}

		opts := manager.EBPFWatchOptions{
			Policy: arg.Policy,
		}

		if arg.RegexPrefilter != "" {
			re, err := regexp.Compile(arg.RegexPrefilter)
			if err != nil {
				scope.Log("watch_ebpf: Unable to compile regex_prefilter %v", err)
				return
			}

			opts.Prefilter = re.Match
		}

		events_chan, closer, err := ebpf_manager.Watch(ctx, opts)
		if err != nil {
			scope.Log("watch_ebpf: %v", err)
			return
		}
		defer closer()

		for {
			select {
			case <-ctx.Done():
				return

			case row := <-events_chan:
				select {
				case <-ctx.Done():
					return

				case output_chan <- enrich(row):
				}
			}
		}
	}()

	return output_chan
}

// The manager is process wide and each instance loads its own copy
// of the eBPF collection. Event artifacts with several sources call
// watch_ebpf() in the same instant, so creation must be serialized
// or every source builds its own manager.
func getEbpfManager(scope vfilter.Scope,
	arg *EBPFEventPluginArgs) (*manager.EBPFManager, error) {
	mu.Lock()
	defer mu.Unlock()

	if gEbpfManager != nil {
		if arg.IncludeEnv && !gEbpfIncludeEnv {
			scope.Log("watch_ebpf: include_env ignored - the eBPF manager was already started without it")
		}
		return gEbpfManager, nil
	}

	config_obj, _ := vql_subsystem.GetServerConfig(scope)
	logger := NewLogger(config_obj)
	logger.SetScope(scope)

	config := manager.Config{
		Options: manager.OptTranslateFDFilePath,
	}
	if arg.IncludeEnv {
		config.Options |= manager.OptExecEnv
	}

	ebpf_manager, err := manager.NewEBPFManager(
		context.Background(), config, logger)
	if err != nil {
		return nil, err
	}

	gEbpfManager = ebpf_manager
	gEbpfIncludeEnv = arg.IncludeEnv
	return ebpf_manager, nil
}

func generateDefaultPolicy(events []string) string {
	var rules []string
	for _, e := range events {
		rules = append(rules, "   - event: "+e)
	}

	return fmt.Sprintf(`
metadata:
   name: policy_%d
spec:
   scope:
     - global
   rules:
`, utils.GetId()) + strings.Join(rules, "\n")
}

type EBPFEventListPlugin struct{}

func (self EBPFEventListPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "ebpf_events",
		Doc:  "Dump information about potential ebpf_events that can be used by the watch_ebpf() plugin",
	}
}

func (self EBPFEventListPlugin) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "watch_ebpf", args)()

		events := manager.GetEvents()
		for _, i := range events.Items() {
			select {
			case <-ctx.Done():
				return

			case output_chan <- ordereddict.NewDict().
				Set("Event", i.Key).
				Set("Metadata", i.Value):
			}
		}

	}()
	return output_chan
}

func init() {
	vql_subsystem.RegisterPlugin(&EBPFEventListPlugin{})
	vql_subsystem.RegisterPlugin(&EBPFEventPlugin{})
}
