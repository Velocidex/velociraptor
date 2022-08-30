package process

import (
	"context"
	"sort"
	"time"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

// The tree consists of nodes of the same format accepted by the GUI's
// more generic "tree" column type.
type node struct {
	Name      string            `json:"name"`
	Id        string            `json:"id"`
	StartTime time.Time         `json:"start_time"`
	Data      *ordereddict.Dict `json:"data"`
	Children  []*node           `json:"children"`
}

type getProcessTreeArgs struct {
	Id           string          `vfilter:"optional,field=id,doc=Process ID."`
	DataCallback *vfilter.Lambda `vfilter:"optional,field=data_callback,doc=A VQL Lambda function to that receives a ProcessEntry and returns the data node for each process."`
}

type getProcessTree struct{}

func (self getProcessTree) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) types.Any {

	arg := &getProcessTreeArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("process_tracker_tree: %v", err)
		return vfilter.Null{}
	}

	tracker := GetGlobalTracker()
	if tracker == nil {
		scope.Log("process_tracker_tree: Initialize a process tracker first with process_tracker_install()")
		return &vfilter.Null{}
	}

	entry, pres := tracker.Enrich(ctx, scope, arg.Id)
	if !pres {
		return &vfilter.Null{}
	}

	new_node := &node{
		Id:        entry.Id,
		Name:      getEntryName(entry),
		StartTime: entry.StartTime,
		Data:      entry.Data,
	}

	seen := make(map[string]bool)
	depth := 0
	getTreeChildren(ctx, scope, new_node, seen, tracker, depth)

	return new_node
}

func getEntryName(entry *ProcessEntry) string {
	if entry.Data != nil {
		name, pres := entry.Data.GetString("Name")
		if pres {
			return name
		}
	}

	return entry.Id
}

func getTreeChildren(
	ctx context.Context, scope vfilter.Scope,
	n *node, seen map[string]bool, tracker IProcessTracker, depth int) {
	if depth > 20 {
		return
	}

	for _, e := range tracker.Children(ctx, scope, n.Id) {
		_, pres := seen[e.Id]
		if pres {
			continue
		}
		seen[e.Id] = true

		// Update these from the process entry
		e.Data.Update("StartTime", e.StartTime)
		e.Data.Update("EndTime", e.EndTime)

		new_node := &node{
			Id:        e.Id,
			Name:      getEntryName(e),
			StartTime: e.StartTime,
			Data:      e.Data,
		}
		n.Children = append(n.Children, new_node)
		getTreeChildren(ctx, scope, new_node, seen, tracker, depth+1)
	}

	// Sort the children by start time
	sort.Slice(n.Children, func(i, j int) bool {
		return n.Children[i].StartTime.Before(n.Children[j].StartTime)
	})
}

func (self getProcessTree) Info(scope types.Scope,
	type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{
		Name:    "process_tracker_tree",
		Doc:     "Get the full process tree under the process id.",
		ArgType: type_map.AddType(scope, &getProcessTreeArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&getProcessTree{})
}
