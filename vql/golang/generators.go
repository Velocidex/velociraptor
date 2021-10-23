package golang

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type Generator struct {
	name                   string
	disable_file_buffering bool
}

// Give Generator the vfilter.StoredQuery interface so it can return
// events.

func (self Generator) Eval(ctx context.Context, scope types.Scope) <-chan types.Row {
	result := make(chan vfilter.Row)

	b, err := services.GetBroadcastService()
	if err != nil {
		scope.Log("generate: %v", err)
		close(result)
		return result
	}

	output_chan, cancel, err := b.Watch(ctx, self.name, directory.QueueOptions{
		DisableFileBuffering: self.disable_file_buffering,
	})

	if err != nil {
		scope.Log("generate: %v", err)
		close(result)
		return result
	}

	go func() {
		defer close(result)
		// Remove the watcher when we are done.
		defer cancel()

		for item := range output_chan {
			select {
			case <-ctx.Done():
				return

			case result <- item:
			}
		}
	}()

	return result
}

type GeneratorArgs struct {
	Name              string            `vfilter:"optional,field=name,doc=Name to call the generator"`
	Query             types.StoredQuery `vfilter:"optional,field=query,doc=Run this query to generator rows."`
	Delay             int64             `vfilter:"optional,field=delay,doc=Wait before starting the query"`
	WithFileBuffering bool              `vfilter:"optional,field=with_file_buffer,doc=Enable file buffering"`
}

type GeneratorFunction struct{}

func (self *GeneratorFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &GeneratorArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("generate: %s", err.Error())
		return false
	}

	if arg.Name == "" {
		arg.Name = types.ToString(arg.Query, scope)
	}

	b, err := services.GetBroadcastService()
	if err != nil {
		scope.Log("generate: %v", err)
		return types.Null{}
	}

	// A channel to send our events on
	generator_chan := make(chan *ordereddict.Dict)

	// Try to register this generator but if it is already registered
	// just wrap the existing one and return it.
	err = b.RegisterGenerator(generator_chan, arg.Name)
	if err == services.AlreadyRegisteredError {
		return Generator{
			name:                   arg.Name,
			disable_file_buffering: !arg.WithFileBuffering,
		}
	}

	scope.Log("generate: registered new query for %v: %v",
		arg.Name, types.ToString(arg.Query, scope))

	sub_ctx, cancel := context.WithCancel(ctx)

	// Remove the generator when the scope destroys.
	scope.AddDestructor(func() {
		scope.Log("generate: Removing generator %v", arg.Name)
		cancel()
	})

	go func() {
		defer close(generator_chan)

		if arg.Delay > 0 {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Duration(arg.Delay) * time.Millisecond):
			}
		}

		for item := range arg.Query.Eval(sub_ctx, scope) {
			select {
			case <-sub_ctx.Done():
				return
			case generator_chan <- vfilter.MaterializedLazyRow(ctx, item, scope):
			}
		}
	}()

	return Generator{
		name:                   arg.Name,
		disable_file_buffering: !arg.WithFileBuffering,
	}
}

func (self GeneratorFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "generate",
		Doc:     "Create a named generator that receives rows from the query.",
		ArgType: type_map.AddType(scope, &GeneratorArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GeneratorFunction{})
}
