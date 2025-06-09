/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package common

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ClockPluginArgs struct {
	StartTime vfilter.Any `vfilter:"optional,field=start,doc=Start at this time."`
	Period    int64       `vfilter:"optional,field=period,doc=Wait this many seconds between events."`
	PeriodMs  int64       `vfilter:"optional,field=ms,doc=Wait this many ms between events."`
}

type ClockPlugin struct{}

func (self ClockPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "clock", args)()

		arg := &ClockPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("clock: %v", err)
			return
		}

		if arg.Period == 0 && arg.PeriodMs == 0 {
			arg.Period = 1
		}

		duration := time.Duration(arg.Period)*time.Second +
			time.Duration(arg.PeriodMs)*time.Second/1000

		if !utils.IsNil(arg.StartTime) {
			start, err := functions.TimeFromAny(ctx, scope, arg.StartTime)
			if err != nil {
				scope.Log("clock: %v", err)
				return
			}

			// Wait for start condition.
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Until(start)):
				output_chan <- time.Now()
			}
		}

		// Now just fire off as normal.
		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(duration):
				output_chan <- time.Now()
			}
		}
	}()

	return output_chan
}

func (self ClockPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "clock",
		Doc: "Generate a timestamp periodically. This is mostly " +
			"useful for event queries.",
		ArgType: type_map.AddType(scope, &ClockPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ClockPlugin{})
}
