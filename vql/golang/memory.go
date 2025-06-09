package golang

import (
	"context"
	"runtime"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type MemoryAllocationsPluginArgs struct{}

type MemoryAllocationsPlugin struct{}

func (self MemoryAllocationsPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "profile_memory", args)()

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("profile_memory: %s", err)
			return
		}

		arg := &MemoryAllocationsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("profile_memory: %s", err.Error())
			return
		}

		var records []runtime.MemProfileRecord

		for i := 0; i < 10; i++ {
			n, ok := runtime.MemProfile(records, false)
			if ok {
				break
			}

			records = make([]runtime.MemProfileRecord, n)
		}

		for _, record := range records {
			output_chan <- ordereddict.NewDict().
				Set("InUseBytes", record.InUseBytes()).
				Set("InUseObjects", record.InUseObjects()).
				Set("CallStack", decodeStack(record.Stack()))
		}
	}()

	return output_chan
}

func (self MemoryAllocationsPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "profile_memory",
		Doc:      "Enumerates all in-use memory within the runtime and tie it to allocation sites.",
		ArgType:  type_map.AddType(scope, &MemoryAllocationsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func decodeStack(stack []uintptr) []*ordereddict.Dict {
	result := make([]*ordereddict.Dict, 0, len(stack))
	frames := runtime.CallersFrames(stack)
	for {
		frame, more := frames.Next()
		name := frame.Function

		if name == "" {
			continue
		}

		result = append(result, ordereddict.NewDict().
			Set("Name", name).
			Set("File", frame.File).
			Set("Line", frame.Line))

		if !more {
			break
		}
	}

	return result
}

func init() {
	vql_subsystem.RegisterPlugin(&MemoryAllocationsPlugin{})
}
