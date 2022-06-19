package process

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
)

// This is the dummy process tracker implementation. It does **not**
// self update but instead invoke the pslist() plugin for fresh data
// each time. This makes it exactly equivalent to just using pslist()
// as before, so it becomes possible to substitute calls to the
// process tracker in all places where previously pslist() was used.

// If a proper process tracker is installed these artifacts will
// suddenly become much more useful.

type DummyProcessTracker struct{}

func (self *DummyProcessTracker) Get(ctx context.Context,
	scope vfilter.Scope, id string) (*ProcessEntry, bool) {

	pslist, pres := scope.GetPlugin("pslist")
	if !pres {
		return nil, false
	}

	pid, err := strconv.ParseInt(id, 0, 64)
	if err == nil {

		for row := range pslist.Call(
			ctx, scope, ordereddict.NewDict().Set("pid", pid)) {
			return getProcessEntry(scope, vfilter.RowToDict(ctx, scope, row))
		}
	}
	return nil, false
}

func (self *DummyProcessTracker) Stats() ttlcache.Metrics {
	return ttlcache.Metrics{}
}

func (self *DummyProcessTracker) Enrich(
	ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool) {
	return self.Get(ctx, scope, id)
}

func (self *DummyProcessTracker) Processes(
	ctx context.Context, scope vfilter.Scope) []*ProcessEntry {

	pslist, pres := scope.GetPlugin("pslist")
	if !pres {
		return nil
	}

	result := []*ProcessEntry{}
	for row := range pslist.Call(ctx, scope, ordereddict.NewDict()) {
		item, ok := getProcessEntry(scope, vfilter.RowToDict(ctx, scope, row))
		if ok {
			result = append(result, item)
		}
	}

	return result
}

func (self *DummyProcessTracker) CallChain(
	ctx context.Context, scope vfilter.Scope, id string) []*ProcessEntry {

	lookup := make(map[string]*ProcessEntry)
	for _, proc := range self.Processes(ctx, scope) {
		lookup[proc.Id] = proc
	}

	result := []*ProcessEntry{}

	for depth := 0; depth < 10; depth++ {
		proc, pres := lookup[id]
		if !pres {
			// Cant find the process return what we have.
			return reverse(result)
		}

		result = append(result, proc)
		id = proc.ParentId
	}

	return reverse(result)
}

func (self *DummyProcessTracker) Children(
	ctx context.Context, scope vfilter.Scope, id string) []*ProcessEntry {

	result := []*ProcessEntry{}
	for _, proc := range self.Processes(ctx, scope) {
		if proc.ParentId == id {
			result = append(result, proc)
		}
	}

	return result
}

func (self *DummyProcessTracker) Updates() chan *ProcessEntry {
	output_chan := make(chan *ProcessEntry)
	close(output_chan)

	return output_chan
}

type ProcessInfoWindows struct {
	Pid       int    `json:"Pid"`
	PPid      int    `json:"PPid"`
	StartTime string `json:"CreateTime"`
}

type ProcessInfoLinux struct {
	Pid       int   `json:"Pid"`
	PPid      int   `json:"Ppid"`
	StartTime int64 `json:"CreateTime"`
}

// Parses the output of various pslist implementations to give a
// ProcessEntry item.
func getProcessEntry(
	scope vfilter.Scope, row *ordereddict.Dict) (*ProcessEntry, bool) {
	serialized, err := row.MarshalJSON()
	if err != nil {
		return nil, false
	}

	windows_item := &ProcessInfoWindows{}
	err = json.Unmarshal(serialized, windows_item)
	if err != nil {
		// Maybe we are running on linux
		unix_item := &ProcessInfoLinux{}
		err = json.Unmarshal(serialized, unix_item)
		if err == nil {

			return &ProcessEntry{
				Id:        fmt.Sprintf("%v", unix_item.Pid),
				ParentId:  fmt.Sprintf("%v", unix_item.PPid),
				StartTime: utils.ParseTimeFromInt64(unix_item.StartTime),
				Data:      row,
			}, true
		}

		return nil, false
	}

	create_time, _ := functions.ParseTimeFromString(scope,
		windows_item.StartTime)

	return &ProcessEntry{
		Id:        fmt.Sprintf("%v", windows_item.Pid),
		ParentId:  fmt.Sprintf("%v", windows_item.PPid),
		StartTime: create_time,
		Data:      row,
	}, true
}
