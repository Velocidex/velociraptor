package process

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
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

type DummyProcessTracker struct {
	mu     sync.Mutex
	lookup map[string]*ProcessEntry
	age    time.Time
}

// Refresh the local cache to avoid having to make too many pslist calls.
func (self *DummyProcessTracker) getLookup(
	ctx context.Context, scope vfilter.Scope) map[string]*ProcessEntry {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.lookup == nil {
		self.lookup = make(map[string]*ProcessEntry)
	}

	// Expire old looksup after 10 seconds
	now := time.Now()
	if now.Before(self.age.Add(10 * time.Second)) {
		return self.lookup
	}

	self.lookup = make(map[string]*ProcessEntry)
	pslist, pres := scope.GetPlugin("pslist")
	if !pres {
		return self.lookup
	}

	self.age = now

	// Different platforms do different things here so we just call
	// the plugin to get the process data.
	for row_any := range pslist.Call(ctx, scope, ordereddict.NewDict()) {
		row, ok := row_any.(*ordereddict.Dict)
		if !ok {
			continue
		}

		entry, pres := getProcessEntry(ctx, scope, row)
		if pres {
			self.lookup[entry.Id] = entry
		}
	}

	return self.lookup
}

func (self *DummyProcessTracker) Get(ctx context.Context,
	scope vfilter.Scope, id string) (*ProcessEntry, bool) {

	lookup := self.getLookup(ctx, scope)
	entry, pres := lookup[id]
	return entry, pres
}

func (self *DummyProcessTracker) Stats() cache.Stats {
	return cache.Stats{}
}

func (self *DummyProcessTracker) Enrich(
	ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool) {
	return self.Get(ctx, scope, id)
}

func (self *DummyProcessTracker) Processes(
	ctx context.Context, scope vfilter.Scope) []*ProcessEntry {

	lookup := self.getLookup(ctx, scope)
	result := []*ProcessEntry{}
	for _, v := range lookup {
		result = append(result, v)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Id < result[j].Id
	})
	return result
}

func (self *DummyProcessTracker) CallChain(
	ctx context.Context, scope vfilter.Scope, id string) []*ProcessEntry {

	lookup := self.getLookup(ctx, scope)
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
func getProcessEntry(ctx context.Context,
	scope vfilter.Scope, row *ordereddict.Dict) (*ProcessEntry, bool) {

	pid, pres := row.GetInt64("Pid")
	if !pres {
		return nil, false
	}

	ppid, pres := row.GetInt64("PPid")
	if !pres {
		return nil, false
	}

	if !pres {
		return nil, false
	}

	create_time_str, pres := row.GetString("CreateTime")
	if !pres {
		return nil, false
	}

	create_time, _ := functions.ParseTimeFromString(
		ctx, scope, create_time_str)

	return &ProcessEntry{
		Id:        fmt.Sprintf("%v", pid),
		ParentId:  fmt.Sprintf("%v", ppid),
		StartTime: create_time,
		Data:      row,
	}, true
}
