package process

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	mu sync.Mutex

	plugin_responses      [][]*ordereddict.Dict
	plugin_responses_idx  = 0
	plugin_responses_sync chan bool

	// Update plugin will signal plugin_update_done when done.
	plugin_update_responses []*ordereddict.Dict
	plugin_update_done      bool
)

type _MockPslist struct {
}

func (self _MockPslist) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "mock_pslist",
	}
}

func (self _MockPslist) Call(
	ctx context.Context, scope types.Scope,
	args *ordereddict.Dict) <-chan types.Row {

	output_chan := make(chan types.Row)

	// Wait for signal here
	mu.Lock()
	w := plugin_responses_sync
	mu.Unlock()

	select {
	case <-w:
	}

	go func() {
		defer close(output_chan)

		mu.Lock()
		responses := plugin_responses[plugin_responses_idx]
		plugin_responses_idx++
		if plugin_responses_idx >= len(plugin_responses) {
			plugin_responses_idx = 0
		}
		mu.Unlock()

		for _, res := range responses {
			select {
			case <-ctx.Done():
				return

			case output_chan <- res:
			}
		}
	}()

	return output_chan
}

func loadMockPlugin(t *testing.T, serialized string) {
	mu.Lock()
	defer mu.Unlock()

	result := make([][]*ordereddict.Dict, 0)

	err := json.Unmarshal([]byte(serialized), &result)
	assert.NoError(t, err)

	plugin_responses = result
	plugin_responses_idx = 0
	plugin_responses_sync = make(chan bool, 1)
	plugin_responses_sync <- true
}

func loadMockUpdatePlugin(t *testing.T, serialized string) {
	mu.Lock()
	defer mu.Unlock()

	result := make([]*ordereddict.Dict, 0)

	err := json.Unmarshal([]byte(serialized), &result)
	assert.NoError(t, err)

	plugin_update_responses = result
	plugin_update_done = false
}

func init() {
	vql_subsystem.RegisterPlugin(&_MockPslist{})

	vql_subsystem.RegisterPlugin(vfilter.GenericListPlugin{
		PluginName: "mock_update",
		Function: func(
			ctx context.Context,
			scope vfilter.Scope,
			args *ordereddict.Dict) []vfilter.Row {
			var result []vfilter.Row

			for _, i := range plugin_update_responses {
				result = append(result, i)
			}
			mu.Lock()

			plugin_update_done = true
			mu.Unlock()

			return result
		}})

	vql_subsystem.RegisterFunction(vfilter.GenericFunction{
		FunctionName: "mock_pslist_next",
		Function: func(
			ctx context.Context,
			scope vfilter.Scope,
			args *ordereddict.Dict) vfilter.Any {

			plugin_responses_sync <- true

			time.Sleep(100 * time.Millisecond)

			return &vfilter.Null{}
		}})

	vql_subsystem.RegisterFunction(vfilter.GenericFunction{
		FunctionName: "mock_update_wait",
		Function: func(
			ctx context.Context,
			scope vfilter.Scope,
			args *ordereddict.Dict) vfilter.Any {

			for {
				select {
				case <-ctx.Done():
					return &vfilter.Null{}

				case <-time.After(100 * time.Millisecond):
					mu.Lock()
					done := plugin_update_done
					mu.Unlock()
					if done {
						return true
					}
				}
			}

			return &vfilter.Null{}
		}})

}
