package vql

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	pluginMonitor PluginMonitor
)

type PluginMonitorEntry struct {
	Name     string
	ArgsFunc func() *ordereddict.Dict
	Start    time.Time

	id uint64
}

type PluginMonitor struct {
	mu      sync.Mutex
	entries map[uint64]*PluginMonitorEntry
}

func (self *PluginMonitor) report(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, item := range self.entries {
		select {
		case <-ctx.Done():
			return

		case output_chan <- ordereddict.NewDict().
			Set("Started", item.Start).
			Set("Plugin", item.Name).
			Set("Args", item.ArgsFunc()).
			Set("Duration", utils.GetTime().Now().Sub(item.Start).String()):
		}
	}
}

func (self *PluginMonitor) Register(name string, args *ordereddict.Dict) func() {
	id := utils.GetId()

	self.mu.Lock()
	defer self.mu.Unlock()

	self.entries[id] = &PluginMonitorEntry{
		Name:     name,
		ArgsFunc: renderArgs(args),
		Start:    utils.GetTime().Now(),
	}

	return func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		delete(self.entries, id)
	}
}

func renderValue(v interface{}) interface{} {
	// Format the args in a safe way to ensure they do not get expanded.
	switch t := v.(type) {
	case string, uint64, uint32, uint16, uint8,
		int64, int32, int16, int8, float64,
		vfilter.Null, *vfilter.Null, bool:
		return t

	case *ordereddict.Dict:
		return renderArgs(t)()

	case types.StringProtocol:
		scope := MakeScope()
		return t.ToString(scope)

	case types.StoredQuery, types.LazyExpr:
		scope := MakeScope()
		return vfilter.FormatToString(scope, t)

	default:
		// Second check based on string type. We can not
		// reference the type directly due to import
		// restrictions.
		type_str := fmt.Sprintf("%T", v)
		switch type_str {
		case "*accessors.OSPath":
			return v
		}
	}
	return fmt.Sprintf("Variable of type %T", v)
}

func renderArgs(args *ordereddict.Dict) func() *ordereddict.Dict {
	return func() *ordereddict.Dict {
		result := ordereddict.NewDict()
		for _, k := range args.Keys() {
			v, _ := args.Get(k)
			result.Set(k, renderValue(v))
		}
		return result
	}
}

func RegisterMonitor(name string, args *ordereddict.Dict) func() {
	return pluginMonitor.Register(name, args)
}

func init() {
	pluginMonitor.entries = make(map[uint64]*PluginMonitorEntry)
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "Plugin Monitor",
		Description:   "See currently running VQL plugins",
		ProfileWriter: pluginMonitor.report,
		Categories:    []string{"Global", "VQL"},
	})
}
