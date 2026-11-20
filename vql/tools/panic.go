package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func messMap(m map[string]bool) {
	for k := range m {
		m[k] = true
	}
}

func makeMapPanic() {
	m := make(map[string]bool)
	for i := 0; i < 100; i++ {
		m[fmt.Sprintf("Key %v", i)] = true
	}

	for i := 0; i < 100; i++ {
		go messMap(m)
	}
}

type PanicPlugin struct{}

func (self PanicPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "panic", args)()

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("panic: %s", err)
			return
		}

		// Allow the user to select what kind of panic should be produced.
		_, pres := args.Get("MapPanic")
		if pres {
			makeMapPanic()
			time.Sleep(10 * time.Second)

		} else {
			panic("oops")
		}
	}()

	return output_chan
}

func (self PanicPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "panic",
		Doc:  "Crash the program with a panic!",
		Metadata: vql_subsystem.VQLMetadata().
			Permissions(acls.SERVER_ADMIN).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&PanicPlugin{})
}
