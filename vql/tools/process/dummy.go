package process

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Velocidex/ordereddict"
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
			return getProcessEntry(vfilter.RowToDict(ctx, scope, row))
		}
	}
	return nil, false
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
		item, ok := getProcessEntry(vfilter.RowToDict(ctx, scope, row))
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

// Parses the output of various pslist implementations to give a
// ProcessEntry item.
func getProcessEntry(row *ordereddict.Dict) (*ProcessEntry, bool) {
	serialized, err := row.MarshalJSON()
	if err != nil {
		return nil, false
	}

	windows_item := &ProcessInfoWindows{}
	err = json.Unmarshal(serialized, windows_item)
	if err != nil {
		return nil, false
	}

	create_time := time.Time{}
	_ = json.Unmarshal([]byte(windows_item.StartTime), &create_time)

	return &ProcessEntry{
		Id:        fmt.Sprintf("%v", windows_item.Pid),
		ParentId:  fmt.Sprintf("%v", windows_item.PPid),
		StartTime: create_time,
		Data:      row,
	}, true
}
