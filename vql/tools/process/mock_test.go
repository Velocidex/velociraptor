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
)

var (
	mu sync.Mutex

	plugin_responses      [][]*ordereddict.Dict
	plugin_responses_idx  = 0
	plugin_responses_sync = make(chan bool)

	// Update plugin will signal plugin_update_done when done.
	plugin_update_responses []*ordereddict.Dict
	plugin_update_done      bool
)

func loadMockPlugin(t *testing.T, serialized string) {
	mu.Lock()
	defer mu.Unlock()

	result := make([][]*ordereddict.Dict, 0)

	err := json.Unmarshal([]byte(serialized), &result)
	assert.NoError(t, err)

	plugin_responses = result
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
	vql_subsystem.RegisterPlugin(vfilter.GenericListPlugin{
		PluginName: "mock_pslist",
		Function: func(
			ctx context.Context,
			scope vfilter.Scope,
			args *ordereddict.Dict) []vfilter.Row {
			var result []vfilter.Row

			mu.Lock()
			defer mu.Unlock()

			if plugin_responses_idx > len(plugin_responses)-1 {
				plugin_responses_idx = 0
			}

			for _, i := range plugin_responses[plugin_responses_idx] {
				result = append(result, i)
			}

			// Try to notify any waiters
			select {
			case plugin_responses_sync <- true:
			default:
			}

			return result
		}})

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
			mu.Lock()
			plugin_responses_idx++
			mu.Unlock()

			// Wait here until the tracker is updated.
			<-plugin_responses_sync

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

				case <-time.After(10 * time.Millisecond):
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
