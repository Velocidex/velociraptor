package common

import (
	"context"
	"time"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	count = 0
)

type ClockPluginArgs struct {
	Period int64 `vfilter:"required,field=period"`
}

type ClockPlugin struct{}

func (self ClockPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	count += 1

	go func() {
		defer close(output_chan)

		arg := &ClockPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("clock: %v", err)
			return
		}

		if arg.Period == 0 {
			arg.Period = 1
		}

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Duration(arg.Period) * time.Second):
				output_chan <- time.Now()
			}
		}
	}()

	return output_chan
}

func (self ClockPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "clock",
		Doc: "Generate a timestamp periodically. This is mostly " +
			"useful for event queries.",
		ArgType: "ClockPluginArgs",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ClockPlugin{})
}
