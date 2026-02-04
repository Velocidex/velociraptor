/*
  Process Tracker is a component which tracks running processes in
  real time. We can query it to resolve parent/child relationships.

  It offers a number of advantages over pure OS API calls:

  1. It is very fast and does not need make API calls.
  2. It keeps track of deleted processes so it can resolve process
     parent/child relationships even when the parent processes exit.
  3. It can build its internal model based on a number of different
     sources, such as ETW, Sysmon or plain API calls.
*/

package process

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/disklru"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	tracker_mu sync.Mutex

	// A global tracker that can be registered with
	// process_tracker_install() and retrieved with process_tracker().
	g_tracker IProcessTracker = &DummyProcessTracker{}
)

func GetGlobalTracker() IProcessTracker {
	tracker_mu.Lock()
	defer tracker_mu.Unlock()

	if g_tracker == nil {
		g_tracker = &DummyProcessTracker{}
	}

	return g_tracker
}

type ProcessTracker struct {
	mu sync.Mutex

	lookup LRUCache

	// Any interested parties will receive notifications of any state
	// updates.
	update_notifications chan *UpdateProcessEntry
	enrichments          []*vfilter.Lambda

	// The last time we did a full sync
	last_full_sync time.Time

	// The maximum number of children to allow under a single process.
	max_children int
}

// Gets the process from cache - also update its TTL.
func (self *ProcessTracker) Get(
	ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool) {
	any, pres := self.lookup.Get(id)
	if !pres {
		return nil, false
	}

	pe, ok := any.(*ProcessEntry)
	if !ok {
		return nil, false
	}

	// If we look up by Pid this might be a link to the real id so get that.
	if pe.RealId != "" {
		res_any, pres := self.lookup.Get(pe.RealId)
		if !pres {
			return nil, false
		}

		res := res_any.(*ProcessEntry)
		self.maybeUpdateEndTime(res)
		return res, true
	}

	self.maybeUpdateEndTime(pe)

	return pe, true
}

func (self *ProcessTracker) maybeUpdateEndTime(entry *ProcessEntry) {
	// If the end time is not set and we have done a full sync since
	// we saw this entry previously, it must have exited. We do not
	// know exactly when so we just estimate the last sync time for
	// the exit time.
	if entry.EndTime.IsZero() && self.last_full_sync.After(entry.LastSyncTime) {
		entry.EndTime = entry.LastSyncTime
	}
}

func (self *ProcessTracker) Peek(
	ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool) {
	any, pres := self.lookup.Peek(id)
	if !pres {
		return nil, false
	}

	pe, ok := any.(*ProcessEntry)
	if !ok {
		return nil, false
	}

	// If we look up by Pid this might be a link to the real id so get that.
	if pe.RealId != "" {
		res_any, pres := self.lookup.Peek(pe.RealId)
		if !pres {
			return nil, false
		}

		res := res_any.(*ProcessEntry)
		self.maybeUpdateEndTime(res)
		return res, true
	}

	self.maybeUpdateEndTime(pe)

	return pe, true
}

func (self *ProcessTracker) Stats() Stats {
	return self.lookup.Stats()
}

func (self *ProcessTracker) Enrich(
	ctx context.Context, scope vfilter.Scope, id string) (*ProcessEntry, bool) {
	pe, pres := self.Get(ctx, scope, id)
	if !pres {
		return nil, false
	}

	for _, enrichment := range self.enrichments {
		update := enrichment.Reduce(ctx, scope, []vfilter.Any{pe})
		update_dict, ok := update.(*ordereddict.Dict)
		if ok {
			for _, i := range update_dict.Items() {
				pe.data.Update(i.Key, i.Value)
			}
		}
	}

	return pe, true
}

func (self *ProcessTracker) Updates() chan *UpdateProcessEntry {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.update_notifications == nil {
		self.update_notifications = make(chan *UpdateProcessEntry)
	}

	return self.update_notifications
}

func (self *ProcessTracker) Processes(
	ctx context.Context, scope vfilter.Scope) []*ProcessEntry {
	res := []*ProcessEntry{}

	self.lookup.HouseKeepOnce()

	for _, item := range self.lookup.Items() {
		pe, ok := item.Value.(*ProcessEntry)
		// Real entries contain a data blob.
		if ok && pe.JSONData != "" {
			res = append(res, pe)
		}
	}

	return res
}

// Return all the processes that are children of this id
func (self *ProcessTracker) Children(
	ctx context.Context, scope vfilter.Scope,
	id string, max_items int64) (res []*ProcessEntry) {

	entry, pres := self.Get(ctx, scope, id)
	if !pres {
		return nil
	}

	for _, child_id := range entry.Children {
		child, pres := self.Get(ctx, scope, child_id)
		if pres {
			res = append(res, child)
		}
		if int64(len(res)) > max_items {
			break
		}
	}

	return res
}

// Get the link from the LRU to the real ID
func (self *ProcessTracker) getRealIdForPid(pid string) (string, bool) {
	link, pres := self.lookup.Peek(pid)
	if !pres || link.(*ProcessEntry).RealId == "" {
		return "", false
	}

	return link.(*ProcessEntry).RealId, true
}

func (self *ProcessTracker) addChildIdToParent(child_id, parent_id string) {
	parent_any, pres := self.lookup.Get(parent_id)
	if pres {
		parent := parent_any.(*ProcessEntry)
		if parent.AddChild(child_id, self.max_children) {
			self.lookup.Set(parent.Id, parent)
		}
	}
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

			update := &UpdateProcessEntry{}
			err := arg_parser.ExtractArgsWithContext(ctx, scope,
				vfilter.RowToDict(ctx, scope, row),
				update)
			if err != nil {
				scope.Log("tracker update query error: %v\n", err)
				continue
			}
			switch update.UpdateType {

			// Fired when a new process starts
			case "start":
				record, err := NewProcessEntryFromUpdate(update)
				if err != nil {
					continue
				}

				// Try to resolve the real parent ID through a link
				parent_id, pres := self.getRealIdForPid(record.ParentId)
				if !pres {
					if !strings.HasSuffix(update.ParentId, "?") {
						record.ParentId = fmt.Sprintf("%v-?", record.ParentId)
					}
				} else {
					record.ParentId = parent_id
				}

				// Add a new process entry
				self.lookup.Set(record.Id, record)

				// Add a link from the bare pid to the real record
				self.lookup.Set(update.Id, &ProcessEntry{
					RealId: record.Id,
				})

				// Now update the parent record in the LRU
				self.addChildIdToParent(record.Id, record.ParentId)

				self.maybeSendUpdate(update)

				// Fired when a process exits - sets the exact time it
				// exited.
			case "exit":
				entry, pres := self.Peek(ctx, scope, update.Id)
				if pres {
					entry.EndTime = update.EndTime
					self.lookup.Set(entry.Id, entry)
				}

				self.maybeSendUpdate(update)
			}
		}
	}
}

func (self *ProcessTracker) doFullSync(
	ctx context.Context, scope vfilter.Scope,
	sync_period time.Duration, vql types.StoredQuery) error {
	subctx, cancel := context.WithTimeout(ctx, sync_period)
	defer cancel()

	all_updates := make(map[string]*ProcessEntry)
	all_updates_dict := ordereddict.NewDict()
	all_links := make(map[string]string)

	for row := range vql.Eval(subctx, scope) {
		update := &UpdateProcessEntry{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope,
			vfilter.RowToDict(ctx, scope, row), update)
		if err != nil {
			return fmt.Errorf("SyncQuery does not return correct rows: %w", err)
		}

		// NOTE: We assume that processes can not be reparented at
		// runtime. This may not be true on all OSs.
		record, err := NewProcessEntryFromUpdate(update)
		if err != nil {
			continue
		}

		all_updates[record.Id] = record

		// A link to the real ID
		all_links[update.Id] = record.Id
	}

	// Second pass we need to resolve the parent pid into real ids.
	for real_id, record := range all_updates {
		// Is the parent in the current live set?
		parent_id, pres := all_links[record.ParentId]
		if pres {
			record.ParentId = parent_id
			continue
		}

		// Do we know about this process already? if so we can re-use
		// the previously discovered parent id.
		known, pres := self.lookup.Peek(real_id)
		if pres {
			// Resolve the real parent id that we already know from
			// the previous record.
			record.ParentId = known.(*ProcessEntry).ParentId
			continue
		}

		// If we get here we do not have a lot of information about
		// the parent process. We can look up the Pid in the LRU to
		// get the real id of the parent, but we have no guarantees
		// that the PID is not reused, which will lead us to associate
		// the wrong parent.

		// This is unfortunately the best we can do.
		parent_link, pres := self.lookup.Peek(record.ParentId)
		if pres {
			record.ParentId = parent_link.(*ProcessEntry).RealId
			continue
		}

		// Mark the parent id as unknown.
		record.ParentId += "-?"
	}

	// In the third pass we update the parent entries with the new
	// child Id
	for real_id, record := range all_updates {
		parent, pres := all_updates[record.ParentId]
		if pres {
			// TODO merge existing children from LRU entry
			if !utils.InString(parent.Children, real_id) {
				parent.Children = append(parent.Children, real_id)
			}
			continue
		}

		// If the parent is not know, try to add the child to the LRU
		// set.
		self.addChildIdToParent(real_id, record.ParentId)
	}

	// Now update the entries for all new full sync entries - these
	// override the known set
	for real_id, record := range all_updates {
		self.lookup.Set(real_id, record)

		// Set the links to the real entry
		self.lookup.Set(record.pid, &ProcessEntry{
			RealId: real_id,
		})

		all_updates_dict.Set(real_id, record)
	}

	self.maybeSendUpdate(&UpdateProcessEntry{
		UpdateType: "sync",
		Data:       all_updates_dict,
	})
	return nil
}

func (self *ProcessTracker) maybeSendUpdate(update *UpdateProcessEntry) {
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
	ctx context.Context, scope vfilter.Scope,
	id string, max_items int64) []*ProcessEntry {

	if max_items == 0 {
		max_items = 10
	}

	result := []*ProcessEntry{}
	for {
		proc, pres := self.Get(ctx, scope, id)
		if !pres {
			return reverse(result)
		}

		// Make a copy so the caller does not need to lock
		proc_copy := *proc
		result = append(result, &proc_copy)
		if int64(len(result)) > max_items {
			return reverse(result)
		}

		if id_seen(proc.ParentId, result) || len(result) > 10 {
			return reverse(result)
		}

		// Look for the parent next
		id = proc.ParentId
	}
}

func NewProcessTracker(
	ctx context.Context,
	scope vfilter.Scope, opts Options) (res *ProcessTracker, err error) {

	var cache LRUCache

	if opts.Filename == "" {
		cache = NewMemoryCache(opts)

	} else {
		cache, err = NewDiskCache(ctx, opts)
		if err != nil {
			return nil, err
		}
	}

	result := &ProcessTracker{
		lookup:       cache,
		max_children: opts.MaxChildren,
	}

	if result.max_children == 0 {
		result.max_children = 10
	}

	return result, nil
}

type _InstallProcessTrackerArgs struct {
	SyncQuery    vfilter.StoredQuery `vfilter:"optional,field=sync_query,doc=Source for full tracker updates. Query must emit rows with the ProcessTrackerUpdate shape - usually uses pslist() to form a full sync."`
	SyncPeriodMs int64               `vfilter:"optional,field=sync_period,doc=How often to do a full sync (default 5000 msec)."`
	UpdateQuery  vfilter.StoredQuery `vfilter:"optional,field=update_query,doc=An Event query that produces live updates of the tracker state."`
	MaxSize      uint64              `vfilter:"optional,field=max_size,doc=Maximum size of process tracker LRU."`
	MaxExpirySec uint64              `vfilter:"optional,field=max_expiry,doc=Expire process records older than this much."`
	Enrichments  []string            `vfilter:"optional,field=enrichments,doc=One or more VQL lambda functions that can enrich the data for the process."`
	CacheFile    string              `vfilter:"optional,field=cache,doc=The path to the cache file - if not set we use a memory based cache."`
}

type _InstallProcessTracker struct{}

func (self _InstallProcessTracker) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "process_tracker", args)()

	arg := &_InstallProcessTrackerArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("process_tracker: %v", err)
		return false
	}

	if arg.SyncPeriodMs == 0 {
		arg.SyncPeriodMs = 5000
	}

	if arg.CacheFile == "" {
		arg.CacheFile = utils.ExpandEnv(vql_subsystem.GetStringFromRow(
			scope, scope, constants.PROCESS_TRACKER_CACHE))
	}

	// The cache contains both process entries and links. The Size
	// should be large enough to also contain live links to the
	// entries.
	opts := Options{
		Options: disklru.Options{
			Filename:             arg.CacheFile,
			MaxSize:              int(arg.MaxSize),
			MaxExpirySec:         int(arg.MaxExpirySec),
			UpdateExpiryOnAccess: true,
			Clock:                LRUClock(0),
			DEBUG: vql_subsystem.GetBoolFromRow(
				scope, scope, constants.LRU_DEBUG),
			ClearOnStart: true,
		},
		MaxChildren: 10}

	if opts.MaxSize == 0 {
		opts.MaxSize = 10000
	}

	if opts.MaxExpirySec == 0 {
		opts.MaxExpirySec = 24 * 60 * 60
	}

	tracker, err := NewProcessTracker(ctx, scope, opts)
	if err != nil {
		scope.Log("ERROR:process_tracker: %v", err)
		return false
	}

	// Add any enrichments to the tracker.
	for _, enrichment := range arg.Enrichments {
		lambda, err := vfilter.ParseLambda(enrichment)
		if err != nil {
			scope.Log("process_tracker: while parsing enrichment %v: %v",
				enrichment, err)
			return false
		}

		tracker.enrichments = append(tracker.enrichments, lambda)
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

				case <-utils.GetTime().After(sync_duration):
					err := tracker.doFullSync(ctx, scope, sync_duration, arg.SyncQuery)
					if err != nil {
						scope.Log("<red>Process_tracker doFullSync</> %v", err)
					}

				}
			}
		}()
	}

	if !utils.IsNil(arg.UpdateQuery) {
		go tracker.doUpdateQuery(ctx, scope, arg.UpdateQuery)
	}

	// Register this tracker as a global tracker.
	tracker_mu.Lock()
	g_tracker = tracker
	tracker_mu.Unlock()

	// When this query is done we remove the process tracker and use
	// the dummy one. This restores the state to the initial state.
	err = vql_subsystem.GetRootScope(scope).AddDestructor(func() {
		tracker_mu.Lock()
		g_tracker = &DummyProcessTracker{}
		tracker_mu.Unlock()

		scope.Log("Uninstalling process tracker.")
	})
	if err != nil {
		scope.Log("InstallProcessTracker: %v", err)
	}
	return tracker
}

func (self *_InstallProcessTracker) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "process_tracker",
		Doc:     "Install a global process tracker.",
		ArgType: type_map.AddType(scope, &_InstallProcessTrackerArgs{}),
		Version: 2,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_InstallProcessTracker{})
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:        "process_tracker",
		Categories:  []string{"Global", "VQL", "Plugins"},
		Description: "Report process tracker stats",
		ProfileWriter: func(ctx context.Context,
			scope vfilter.Scope, output_chan chan vfilter.Row) {
			output_chan <- ordereddict.NewDict().
				Set("Type", "process_tracker").
				Set("Line", GetGlobalTracker().Stats())
		},
	})

}
