package golang

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"runtime/pprof"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GoRoutinesPluginArgs struct {
	Verbose bool `vfilter:"optional,field=verbose,doc=Emit information in verbose form."`
}

type GoRoutinesPlugin struct{}

func (self GoRoutinesPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "profile_goroutines", args)()

		defer utils.RecoverVQL(scope)

		err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
		if err != nil {
			scope.Log("profile_goroutines: %s", err)
			return
		}

		arg := &GoRoutinesPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("profile_goroutines: %s", err.Error())
			return
		}

		profile := pprof.Lookup("goroutine")
		if profile != nil {
			buf := bytes.Buffer{}
			err := profile.WriteTo(&buf, 0)
			if err == nil {
				// Buf is compressed - decompress it now.
				reader, err := gzip.NewReader(bytes.NewBuffer(buf.Bytes()))
				if err != nil {
					return
				}

				cleartext, err := utils.ReadAllWithLimit(reader,
					constants.MAX_MEMORY)
				if err != nil {
					return
				}

				// Parse out the profile information
				profile := Profile{}

				err = proto.Unmarshal(cleartext, &profile)
				if err != nil {
					return
				}
				PrintProfile(&profile, output_chan, arg.Verbose)
			}
		}
	}()

	return output_chan
}

// The profile protobuf is compressed internally so we need to do some
// gymnastics to decode it properly.
func PrintProfile(profile *Profile, output_chan chan vfilter.Row, verbose bool) {
	if verbose {
		for _, sample := range profile.Sample {
			stack := make([]*ordereddict.Dict, 0, len(sample.LocationId))
			for _, l := range sample.LocationId {
				location := profile.Location[l-1]
				row := ordereddict.NewDict().
					Set("Line", location.Line[0].Line)
				func_obj := profile.Function[location.Line[0].FunctionId-1]

				row.Set("Name", profile.StringTable[func_obj.Name])
				row.Set("File", profile.StringTable[func_obj.Filename])

				stack = append(stack, row)
			}
			output_chan <- ordereddict.NewDict().
				Set("Count", sample.Value[0]).
				Set("CallStack", stack)
		}

	} else {

		// Concise output - entries on one line.
		for _, sample := range profile.Sample {
			stack := make([]string, 0, len(sample.LocationId))
			for _, l := range sample.LocationId {
				location := profile.Location[l-1]
				func_obj := profile.Function[location.Line[0].FunctionId-1]

				stack = append(stack, fmt.Sprintf("%v %v:%v",
					profile.StringTable[func_obj.Name],
					profile.StringTable[func_obj.Filename],
					location.Line[0].Line))
			}

			output_chan <- ordereddict.NewDict().
				Set("Count", sample.Value[0]).
				Set("CallStack", stack)
		}
	}
}

func (self GoRoutinesPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "profile_goroutines",
		Doc:      "Enumerates all running goroutines.",
		ArgType:  type_map.AddType(scope, &GoRoutinesPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&GoRoutinesPlugin{})
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:        "goroutines",
		Description: "Goroutine information",
		Categories:  []string{"Global"},
		ProfileWriter: func(
			ctx context.Context, scope vfilter.Scope, output_chan chan vfilter.Row) {
			plugin := GoRoutinesPlugin{}
			for row := range plugin.Call(
				ctx, scope, ordereddict.NewDict()) {
				output_chan <- row
			}
		},
	})

	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:        "verbose goroutines",
		Description: "Goroutine information (Verbose)",
		Categories:  []string{"Global"},
		ProfileWriter: func(
			ctx context.Context, scope vfilter.Scope, output_chan chan vfilter.Row) {
			plugin := GoRoutinesPlugin{}
			for row := range plugin.Call(
				ctx, scope, ordereddict.NewDict().Set("verbose", true)) {
				output_chan <- row
			}
		},
	})

}
