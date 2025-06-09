package common

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type BatchPluginArgs struct {
	BatchSize int64               `vfilter:"optional,field=batch_size,doc=Size of batch (defaults to 10)."`
	BatchFunc string              `vfilter:"optional,field=batch_func,doc=A VQL Lambda that determines when a batch is ready. Example 'x=>len(list=x) >= 10'."`
	Query     vfilter.StoredQuery `vfilter:"required,field=query,doc=Run this query over the item."`
	Timeout   uint64              `vfilter:"optional,field=timeout,doc=If specified we flush incomplete batches in this many seconds."`
}

type BatchPlugin struct{}

func (self BatchPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "batch", args)()

		arg := &BatchPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("batch: %v", err)
			return
		}

		if arg.Timeout == 0 {
			arg.Timeout = 3600
		}

		var lambda *vfilter.Lambda

		if arg.BatchFunc != "" {
			// Compile the batch lambda.
			lambda, err = vfilter.ParseLambda(arg.BatchFunc)
			if err != nil {
				scope.Log("batch: %v", err)
				return
			}

		} else if arg.BatchSize == 0 {
			arg.BatchSize = 10
		}

		// Accumulate rows into this batch
		rows := []vfilter.Row{}

		alarm := time.NewTimer(time.Second * time.Duration(arg.Timeout))

		send_batch := func() {
			alarm.Reset(time.Second * time.Duration(arg.Timeout))

			if len(rows) > 0 {
				select {
				case <-ctx.Done():
					return

				case output_chan <- ordereddict.NewDict().Set("rows", rows):
					rows = nil
				}
			}
		}

		// When we are done send whatever is left anyway.
		defer send_batch()

		event_chan := arg.Query.Eval(ctx, scope)

		for {
			select {
			case <-ctx.Done():
				return

				// If the alarm fires we send the batch and reset it.
			case <-alarm.C:
				send_batch()

			case item, ok := <-event_chan:
				if !ok {
					return
				}
				rows = append(rows, item)

				// Handle fixed batch size
				if arg.BatchSize > 0 {
					if len(rows) >= int(arg.BatchSize) {
						send_batch()
					}

				} else {
					// Handle a batch function
					if scope.Bool(lambda.Reduce(ctx, scope, []types.Any{rows})) {
						send_batch()
					}
				}
			}
		}
	}()

	return output_chan
}

func (self BatchPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "batch",
		Doc:     "Batches query rows into multiple arrays.",
		ArgType: type_map.AddType(scope, &BatchPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&BatchPlugin{})
}
