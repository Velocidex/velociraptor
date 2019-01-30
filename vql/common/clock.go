/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	count = 0
)

type ClockPluginArgs struct {
	Period   int64 `vfilter:"optional,field=period,doc=Wait this many seconds between events."`
	PeriodMs int64 `vfilter:"optional,field=ms,doc=Wait this many ms between events."`
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

		if arg.Period == 0 && arg.PeriodMs == 0 {
			arg.Period = 1
		}

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(
				time.Duration(arg.Period)*time.Second +
					time.Duration(arg.PeriodMs)*
						time.Second/1000):
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
		ArgType: type_map.AddType(scope, &ClockPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&ClockPlugin{})
}
