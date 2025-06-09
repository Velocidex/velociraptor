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

/* Delay Plugin

This plugin introduces a delay to an events query such that rows will
be relayed no sooner than the specified delay.

It is needed to ensure some event sources have been processes before
others.

*/

package tools

import (
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type container struct {
	row vfilter.Row
	due time.Time
}

type DelayPluginArgs struct {
	Query    vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for rows."`
	DelaySec int64               `vfilter:"required,field=delay,doc=Number of seconds to delay."`
	Size     int64               `vfilter:"optional,field=buffer_size,doc=Maximum number of rows to buffer (default 1000)."`
}

type DelayPlugin struct{}

func (self DelayPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	sub_ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer vql_subsystem.RegisterMonitor(ctx, "delay", args)()
		defer cancel()

		wg := &sync.WaitGroup{}
		defer wg.Wait()

		arg := &DelayPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(sub_ctx, scope, args, arg)
		if err != nil {
			scope.Log("delay: %v", err)
			return
		}

		if arg.Size == 0 {
			arg.Size = 1000
		}

		if arg.Size > 1000000 {
			arg.Size = 1000000
		}

		buffer := make(chan *container, arg.Size)

		// This routine pumps data from the buffer to the output_chan
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(output_chan)

			for {
				select {
				case <-sub_ctx.Done():
					return

				case row_container, ok := <-buffer:
					if !ok {
						return
					}

					now := utils.GetTime().Now()

					if row_container.due.After(now) {
						// Wait until it is time.
						utils.SleepWithCtx(sub_ctx, row_container.due.Sub(now))
					}

					select {
					case <-sub_ctx.Done():
						return

					case output_chan <- row_container.row:
					}
				}
			}
		}()

		// This routine pumps data from the input query to the buffer.
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(buffer)

			delay := time.Second * time.Duration(arg.DelaySec)

			row_chan := arg.Query.Eval(sub_ctx, scope)
			for {
				select {
				case <-sub_ctx.Done():
					return

				case row, ok := <-row_chan:
					if !ok {
						return
					}

					event := &container{
						row: row,
						due: utils.GetTime().Now().Add(delay),
					}
					select {
					case <-sub_ctx.Done():
						return
					case buffer <- event:
					}
				}
			}
		}()
	}()

	return output_chan
}

func (self DelayPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "delay",
		Doc:  "Executes 'query' and delays relaying the rows by the specified number of seconds.",

		ArgType: type_map.AddType(scope, &DelayPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&DelayPlugin{})
}
