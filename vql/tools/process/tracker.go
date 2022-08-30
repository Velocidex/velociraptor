/*
  Process Tracker is a component which tracks running processes in
  real time. We can query it to resolve parent/child relationships.

  It offers a number of advantages over pure OS API calls:

  1. It is very fast and does not need make API calls.
  2. It keeps track of deleted processes so it can resolve process
     parent/child relationships even when the parent processes exit.
  3. It can build it's internal model based on a number of different
     sources, such as ETW, Sysmon or plain API calls.
*/

package process

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	clock_mu sync.Mutex

	// A global tracker that can be registered with
	// process_tracker_install() and retrieved with process_tracker().
	g_tracker IProcessTracker = &DummyProcessTracker{}
	clock     utils.Clock     = &utils.RealClock{}
)

func GetGlobalTracker() IProcessTracker {
	clock_mu.Lock()
	defer clock_mu.Unlock()

	return g_tracker
}

func GetClock() utils.Clock {
	clock_mu.Lock()
	defer clock_mu.Unlock()

	return clock
}

func SetClock(c utils.Clock) {
	clock_mu.Lock()
	defer clock_mu.Unlock()

	clock = c
}

type ProcessTracker struct {
	mu     sync.Mutex
	lookup *ttlcache.Cache // map[string]*ProcessEntry

	// Any interested parties will receive notifications of any state
	// updates.
	update_notifications chan *ProcessEntry

	enrichments *ordereddict.Dict // map[string]*vfilter.Lambda
}

func (self *ProcessTracker) Get(
	ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool) {
	return self.get(id)
}

func (self *ProcessTracker) get(id string) (*ProcessEntry, bool) {
	any, err := self.lookup.Get(id)
	if err != nil {
		return nil, false
	}

	pe, ok := any.(*ProcessEntry)
	if !ok {
		return nil, false
	}
	return pe, true
}

func (self *ProcessTracker) SetEntry(id string, entry *ProcessEntry) (old_id string, pres bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Get the existing entry for this id if it exists
	item, pres := self.get(id)

	// No existing entry - just set it
	if !pres || item == nil {
		self.lookup.Set(id, entry)
		return id, false
	}

	// Use the start time to identify the same process entry. If the
	// start times are identical (within one second) then it is the
	// same record.
	time_diff := item.StartTime.Unix() - entry.StartTime.Unix()
	if item.Id == entry.Id && time_diff < 2 && time_diff > -2 {
		entry.ParentId = item.ParentId
		self.lookup.Set(id, entry)
		return id, false
	}

	// We are here when the records have the same pid and different
	// create times. This means they are not the same process and pid
	// is reused. In this case we change the id of the old process
	// record and set the new record. The new id is a combination of
	// pid and start time.

	// Once the old ID is migrated, lookups for the pid will not fetch
	// the old entry, but will get the new entry instead. However any
	// parent/child relations will still refer to the old ID by its
	// unique ID.
	new_id := fmt.Sprintf("%v-%v", id, item.StartTime.Unix())
	self.moveId(id, new_id)

	// Set the new entry as the same id
	self.lookup.Set(id, entry)

	// Indicate the old record's ID to our caller
	return new_id, true
}

// Sweep through the entire tracker and replace all referencee from
// the old id to the new id.
func (self *ProcessTracker) moveId(old_id, new_id string) {
	for _, k := range self.lookup.GetKeys() {
		any, err := self.lookup.Get(k)
		if err != nil || any == nil {
			continue
		}

		pe, ok := any.(*ProcessEntry)
		if !ok {
			continue
		}

		// Update the IDs as needed.
		if pe.Id == old_id {
			pe.Id = new_id

			// Set the entry under the new key in the LRU
			self.lookup.Remove(old_id)
			self.lookup.Set(new_id, pe)
		}

		if pe.ParentId == old_id {
			pe.ParentId = new_id
		}
	}
}

func (self *ProcessTracker) Stats() ttlcache.Metrics {
	return self.lookup.GetMetrics()
}

func (self *ProcessTracker) Enrich(
	ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool) {
	any, err := self.lookup.Get(id)
	if err != nil {
		return nil, false
	}

	pe, ok := any.(*ProcessEntry)
	if !ok {
		return nil, false
	}

	for _, k := range self.enrichments.Keys() {
		enrichment_any, _ := self.enrichments.Get(k)
		enrichment, ok := enrichment_any.(*vfilter.Lambda)
		if !ok {
			continue
		}
		update := enrichment.Reduce(ctx, scope, []vfilter.Any{pe})
		update_dict, ok := update.(*ordereddict.Dict)
		if ok {
			for _, k := range update_dict.Keys() {
				v, _ := update_dict.Get(k)
				pe.Data.Update(k, v)
			}
		}
	}

	return pe, true
}

func (self *ProcessTracker) Updates() chan *ProcessEntry {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.update_notifications == nil {
		self.update_notifications = make(chan *ProcessEntry)
	}

	return self.update_notifications
}

func (self *ProcessTracker) Processes(
	ctx context.Context, scope vfilter.Scope) []*ProcessEntry {
	res := []*ProcessEntry{}

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, id := range self.lookup.GetKeys() {
		v, pres := self.Get(ctx, scope, id)
		if pres {
			res = append(res, v)
		}
	}

	return res
}

// Return all the processes that are children of this id
func (self *ProcessTracker) Children(
	ctx context.Context, scope vfilter.Scope, id string) []*ProcessEntry {
	res := []*ProcessEntry{}

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, item_id := range self.lookup.GetKeys() {
		v, pres := self.Get(ctx, scope, item_id)
		if pres {
			if v.ParentId == id {
				res = append(res, v)
			}
		}
	}

	return res
}

func (self *ProcessTracker) doUpdateQuery(
	ctx context.Context, scope vfilter.Scope,
	vql types.StoredQuery) {

	row_chan := vql.Eval(ctx, scope)
	for {
		select {
		case <-ctx.Done():
			return

		case row, ok := <-row_chan:
			if !ok {
				return
			}

			update := &ProcessEntry{}
			err := arg_parser.ExtractArgsWithContext(ctx, scope,
				vfilter.RowToDict(ctx, scope, row),
				update)
			if err != nil {
				scope.Log("tracker update query error: %v\n", err)
				continue
			}
			switch update.UpdateType {
			case "start":
				self.maybeSendUpdate(update)
				self.SetEntry(update.Id, &ProcessEntry{
					StartTime: update.StartTime,
					Id:        update.Id,
					ParentId:  update.ParentId,
					Data:      update.Data,
				})

			case "exit":
				self.maybeSendUpdate(update)

				self.mu.Lock()
				entry, pres := self.Get(ctx, scope, update.Id)
				if pres {
					entry.EndTime = update.EndTime
					self.lookup.Set(update.Id, entry)
				}
				self.mu.Unlock()
			}
		}
	}
}

func (self *ProcessTracker) doFullSync(
	ctx context.Context, scope vfilter.Scope,
	sync_period time.Duration, vql types.StoredQuery) error {
	subctx, cancel := context.WithTimeout(ctx, sync_period)
	defer cancel()

	now := GetClock().Now().UTC()
	all_updates := ordereddict.NewDict()

	for row := range vql.Eval(subctx, scope) {
		update := &ProcessEntry{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope,
			vfilter.RowToDict(ctx, scope, row),
			update)
		if err != nil {
			return fmt.Errorf("SyncQuery does not return correct rows: %w", err)
		}

		self.SetEntry(update.Id, update)
		all_updates.Set(update.Id, update)
	}

	// Now go over all the existing processes in the tracker and if we
	// have not just updated them, then we assume they were exited so
	// we update exit time if needed.
	for _, id := range self.lookup.GetKeys() {
		// Set the exit time if the process still exists.
		item, pres := self.get(id)
		if !pres || !item.EndTime.IsZero() {
			continue
		}

		// Check if we just updated the entry.
		_, pres = all_updates.Get(id)
		if !pres {
			// No we have not seen this entry and it has no end time
			// set - update that now.
			item.EndTime = now
			self.lookup.Set(id, item)
		}

		// If a process has no valid parent at this time we must mark
		// its parent id so as to prevent a new process with the same
		// pid being added in future and clashing with it.
		if !strings.Contains(item.ParentId, "-") {
			_, pres := self.get(item.ParentId)
			if !pres {
				item.ParentId = fmt.Sprintf("%s-?", item.ParentId)
			}
		}
	}

	self.maybeSendUpdate(&ProcessEntry{
		UpdateType: "sync",
		Data:       all_updates,
	})
	return nil
}

func (self *ProcessTracker) maybeSendUpdate(update *ProcessEntry) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.update_notifications == nil {
		return
	}

	// Do not block at all - we can not wait to update our model.
	select {
	case self.update_notifications <- update:
	default:
	}
}

func (self *ProcessTracker) CallChain(
	ctx context.Context, scope vfilter.Scope, id string) []*ProcessEntry {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []*ProcessEntry{}
	for {
		proc, pres := self.Get(ctx, scope, id)
		if !pres {
			return reverse(result)
		}

		// Make a copy so the caller does not need to lock
		proc_copy := *proc
		result = append(result, &proc_copy)

		id = proc.ParentId
		if id_seen(id, result) || len(result) > 10 {
			return reverse(result)
		}
	}
}

func NewProcessTracker(max_size int) *ProcessTracker {
	result := &ProcessTracker{
		lookup:      ttlcache.NewCache(),
		enrichments: ordereddict.NewDict(),
	}

	result.lookup.SetCacheSizeLimit(max_size)

	return result
}

type ProcessEntry struct {
	Id           string            `vfilter:"required,field=id,doc=Process ID."`
	ParentId     string            `vfilter:"optional,field=parent_id,doc=The parent's process ID."`
	RealParentId string            `vfilter:"optional,field=real_parent_id,doc=The parent's real process ID."`
	UpdateType   string            `vfilter:"optional,field=update_type,doc=What this row represents."`
	StartTime    time.Time         `vfilter:"optional,field=start_time,doc=Timestamp for start,end updates"`
	EndTime      time.Time         `vfilter:"optional,field=end_time,doc=Timestamp for start,end updates"`
	Data         *ordereddict.Dict `vfilter:"optional,field=data,doc=Arbitrary key/value to associate with the process"`
}

type _InstallProcessTrackerArgs struct {
	SyncQuery    vfilter.StoredQuery `vfilter:"optional,field=sync_query,doc=Source for full tracker updates. Query must emit rows with the ProcessTrackerUpdate shape - usually uses pslist() to form a full sync."`
	SyncPeriodMs int64               `vfilter:"optional,field=sync_period,doc=How often to do a full sync (default 5000 msec)."`
	UpdateQuery  vfilter.StoredQuery `vfilter:"optional,field=update_query,doc=An Event query that produces live updates of the tracker state."`
	MaxSize      int64               `vfilter:"optional,field=max_size,doc=Maximum size of process tracker LRU."`
	Enrichments  []string            `vfilter:"optional,field=enrichments,doc=One or more VQL lambda functions that can enrich the data for the process."`
}

type _InstallProcessTracker struct{}

func (self _InstallProcessTracker) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &_InstallProcessTrackerArgs{}

	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("process_tracker: %v", err)
		return false
	}

	if arg.SyncPeriodMs == 0 {
		arg.SyncPeriodMs = 5000
	}

	max_size := arg.MaxSize
	if max_size == 0 {
		max_size = 10000
	}

	tracker := NewProcessTracker(int(max_size))

	// Any any enrichments to the tracker.
	for _, enrichment := range arg.Enrichments {
		lambda, err := vfilter.ParseLambda(enrichment)
		if err != nil {
			scope.Log("process_tracker: while parsing enrichment %v: %v",
				enrichment, err)
			return false
		}

		tracker.enrichments.Set(enrichment, lambda)
	}

	sync_duration := time.Duration(arg.SyncPeriodMs) * time.Millisecond

	if !utils.IsNil(arg.SyncQuery) {
		// Do the first sync inline so we are all ready when we return.
		err = tracker.doFullSync(ctx, scope, sync_duration, arg.SyncQuery)
		if err != nil {
			scope.Log("process_tracker: %v", err)
			return false
		}

		// Run the sync query to refresh the tracker periodically.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return

				case <-GetClock().After(sync_duration):
					tracker.doFullSync(ctx, scope, sync_duration, arg.SyncQuery)
				}
			}
		}()
	}

	if !utils.IsNil(arg.UpdateQuery) {
		go tracker.doUpdateQuery(ctx, scope, arg.UpdateQuery)
	}

	// Register this tracker as a global tracker.
	clock_mu.Lock()
	g_tracker = tracker
	clock_mu.Unlock()

	// When this query is done we remove the process tracker and use
	// the dummy one. This restores the state to the initial state.
	vql_subsystem.GetRootScope(scope).AddDestructor(func() {
		clock_mu.Lock()
		g_tracker = &DummyProcessTracker{}
		clock_mu.Unlock()

		scope.Log("Uninstalling process tracker.")
	})

	return tracker
}

func (self *_InstallProcessTracker) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "process_tracker",
		Doc:     "Install a global process tracker.",
		ArgType: type_map.AddType(scope, &_InstallProcessTrackerArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_InstallProcessTracker{})
}
