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
	g_tracker *ProcessTracker = NewProcessTracker(1000)
	clock     utils.Clock     = &utils.RealClock{}
)

func GetGlobalTracker() *ProcessTracker {
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
}

func (self *ProcessTracker) Get(id string) (*ProcessEntry, bool) {
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

func (self *ProcessTracker) Processes() []*ProcessEntry {
	res := []*ProcessEntry{}

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, id := range self.lookup.GetKeys() {
		v, pres := self.Get(id)
		if pres {
			res = append(res, v)
		}
	}

	return res
}

// Return all the processes that are children of this id
func (self *ProcessTracker) Children(id string) []*ProcessEntry {
	res := []*ProcessEntry{}

	self.mu.Lock()
	defer self.mu.Unlock()

	for _, item_id := range self.lookup.GetKeys() {
		v, pres := self.Get(item_id)
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
				self.mu.Lock()
				self.lookup.Set(update.Id, &ProcessEntry{
					StartTime: update.StartTime,
					Id:        update.Id,
					ParentId:  update.ParentId,
					Data:      update.Data,
				})
				self.mu.Unlock()

			case "exit":
				self.maybeSendUpdate(update)

				self.mu.Lock()
				entry, pres := self.Get(update.Id)
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

	now := GetClock().Now()
	existing := make(map[string]bool)
	self.mu.Lock()
	for _, k := range self.lookup.GetKeys() {
		existing[k] = true
	}
	self.mu.Unlock()

	all_updates := []*ProcessEntry{}

	for row := range vql.Eval(subctx, scope) {
		update := &ProcessEntry{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope,
			vfilter.RowToDict(ctx, scope, row),
			update)
		if err != nil {
			return fmt.Errorf("SyncQuery does not return correct rows: %w", err)
		}

		self.mu.Lock()
		self.lookup.Set(update.Id, update)
		delete(existing, update.Id)
		self.mu.Unlock()

		all_updates = append(all_updates, update)
	}

	// Now go over all the existing processes which were not found and
	// update exit time if needed.
	self.mu.Lock()
	for id := range existing {
		item, pres := self.Get(id)
		if pres {
			item.EndTime = now
		}
		self.lookup.Set(id, item)
	}
	self.mu.Unlock()

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

func (self *ProcessTracker) CallChain(id string) []*ProcessEntry {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []*ProcessEntry{}
	for {
		proc, pres := self.Get(id)
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
		lookup: ttlcache.NewCache(),
	}

	result.lookup.SetCacheSizeLimit(max_size)

	return result
}

type ProcessEntry struct {
	Id           string      `vfilter:"required,field=id,doc=Process ID."`
	ParentId     string      `vfilter:"optional,field=parent_id,doc=The parent's process ID."`
	RealParentId string      `vfilter:"optional,field=real_parent_id,doc=The parent's real process ID."`
	UpdateType   string      `vfilter:"optional,field=update_type,doc=What this row represents."`
	StartTime    time.Time   `vfilter:"optional,field=start_time,doc=Timestamp for start,end updates"`
	EndTime      time.Time   `vfilter:"optional,field=end_time,doc=Timestamp for start,end updates"`
	Data         vfilter.Any `vfilter:"optional,field=data,doc=Arbitrary key/value to associate with the process"`
}

type _InstallProcessTrackerArgs struct {
	SyncQuery    vfilter.StoredQuery `vfilter:"required,field=sync_query,doc=Source for full tracker updates. Query must emit rows with the ProcessTrackerUpdate shape - usually uses pslist() to form a full sync."`
	SyncPeriodMs int64               `vfilter:"optional,field=sync_period,doc=How often to do a full sync (default 5000 msec)."`
	UpdateQuery  vfilter.StoredQuery `vfilter:"optional,field=update_query,doc=An Event query that produces live updates of the tracker state."`
	MaxSize      int64               `vfilter:"optional,field=max_size,doc=Maximum size of process tracker LRU."`
}

type _InstallProcessTracker struct{}

func (self _InstallProcessTracker) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &_InstallProcessTrackerArgs{}

	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("process_tracker: %s", err.Error())
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
	sync_duration := time.Duration(arg.SyncPeriodMs) * time.Millisecond

	if !utils.IsNil(arg.SyncQuery) {
		// Do the first sync inline so we are all ready when we return.
		tracker.doFullSync(ctx, scope, sync_duration, arg.SyncQuery)

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
	defer clock_mu.Unlock()

	g_tracker = tracker

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
