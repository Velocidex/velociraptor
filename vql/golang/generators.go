package golang

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type Generator struct {
	name                   string
	description            string
	disable_file_buffering bool
}

// Give Generator the vfilter.StoredQuery interface so it can return
// events.
func (self Generator) Eval(ctx context.Context, scope types.Scope) <-chan types.Row {
	result := make(chan vfilter.Row)

	go func() {
		defer close(result)

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("generate: Command can only run on the server")
			return
		}

		b, err := services.GetBroadcastService(config_obj)
		if err != nil {
			scope.Log("generate: %v", err)
			return
		}

		output_chan, cancel, err := b.Watch(ctx, self.name, api.QueueOptions{
			DisableFileBuffering: self.disable_file_buffering,
			OwnerName:            self.description,
		})
		if err != nil {
			scope.Log("generate: %v", err)
			return
		}

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
	FanOut            int64             `vfilter:"optional,field=fan_out,doc=Wait for this many listeners to connect before starting the query"`
	Description       string            `vfilter:"optional,field=description,doc=A description to add to debug server"`
}

type GeneratorFunction struct{}

func (self *GeneratorFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "generate", args)()

	arg := &GeneratorArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("generate: %s", err.Error())
		return false
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("generate: Command can only run on the server")
		return false
	}

	if arg.Name == "" {
		arg.Name = vfilter.FormatToString(scope, arg.Query)
	}

	b, err := services.GetBroadcastService(config_obj)
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
			description:            arg.Description,
			disable_file_buffering: !arg.WithFileBuffering,
		}
	}

	scope.Log("generate: registered new query for %v", arg.Name)

	sub_ctx, cancel := context.WithCancel(ctx)

	// Remove the generator when the scope destroys.
	err = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
		scope.Log("generate: Removing generator %v", arg.Name)
		cancel()
	})
	if err != nil {
		scope.Log("generate: %v", err)
		cancel()
	}

	go func() {
		defer close(generator_chan)

		if arg.Delay > 0 {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Duration(arg.Delay) * time.Millisecond):
			}
		}

		if arg.FanOut > 0 {
			b.WaitForListeners(sub_ctx, arg.Name, arg.FanOut)
		}

		for item := range arg.Query.Eval(sub_ctx, scope) {
			materialized := vfilter.MaterializedLazyRow(ctx, item, scope)
			select {
			case <-sub_ctx.Done():
				return
			case generator_chan <- materialized:
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
		Version: 2,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GeneratorFunction{})
}
