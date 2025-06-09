/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

/*
	Plugin FIFO.

The fifo plugin collects the more recent rows from a subquery and holds
them in memory. Subsequent calls to it will return the entire cached
row set.

This is useful when the subquery is an event query. Querying the
fifo() plugin allows queries to operate on the most recent events as a
group (For example, what were the last X failed logon events).

Note that the fifo() lifetime is tied with the root scope - it will
remain active for the entire query duration but will be torn down when
the query is cancelled.
*/
package common

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _FIFOCacheEntry struct {
	row  vfilter.Row
	time time.Time
}
type _FIFOCache struct {
	mu sync.Mutex

	rows  *list.List
	count int64

	max_time time.Duration
	max_rows int64
}

func (self *_FIFOCache) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.rows = list.New()
	self.count = 0
}

func (self *_FIFOCache) Snapshot(flush bool) []vfilter.Row {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := []vfilter.Row{}
	for idx := self.rows.Front(); idx != nil; idx = idx.Next() {
		entry := idx.Value.(*_FIFOCacheEntry)
		result = append(result, entry.row)
	}

	// Flush the cache while we still have a lock on it.
	if flush {
		self.rows = list.New()
		self.count = 0
	}

	return result
}

func (self *_FIFOCache) Push(row vfilter.Row) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Push(row)
}

func (self *_FIFOCache) _Push(row vfilter.Row) {
	// First add the entry to the back.
	self.count += 1
	self.rows.PushBack(&_FIFOCacheEntry{
		row:  row,
		time: time.Now(),
	})

	// expire any items which are too old or if we have too many
	// items.
	for e := self.rows.Front(); e != nil; e = e.Next() {
		if self.count > self.max_rows {
			self.rows.Remove(e)
			self.count -= 1
			continue
		}

		// Inspect the cache entry for validity.
		entry, ok := e.Value.(*_FIFOCacheEntry)
		if ok {
			// Item expired, remove it.
			if time.Now().After(entry.time.Add(self.max_time)) {
				self.rows.Remove(e)
				self.count -= 1
				continue
			}
		}
	}
}

func NewFIFOCache(
	ctx context.Context,
	scope vfilter.Scope,
	max_time time.Duration,
	max_rows int64,
	stored_query vfilter.StoredQuery) *_FIFOCache {
	result := &_FIFOCache{
		rows:     list.New(),
		max_time: max_time,
		max_rows: max_rows,
	}

	done := make(chan bool)
	err := vql_subsystem.GetRootScope(scope).AddDestructor(func() {
		close(done)
	})
	if err != nil {
		scope.Log("AddDestructor: %s", err)
		close(done)
	}

	// Start the query and populate the _FIFOCache.
	go func() {
		// Create a backgroud context to ensure this query
		// keeps running *after* we are finished. It will
		// eventually be destroyed when the root scope is
		// done.
		subctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		row_chan := stored_query.Eval(subctx, scope)
		for {
			select {
			case row, ok := <-row_chan:
				if !ok {
					return
				}
				result.Push(row)
			case <-done: // Scope is destroyed, cancel.
				return

			}
		}
	}()

	return result
}

type _FIFOPluginArgs struct {
	Query   vfilter.StoredQuery `vfilter:"required,field=query,doc=Source for cached rows."`
	MaxAge  int64               `vfilter:"optional,field=max_age,doc=Maximum number of seconds to hold rows in the fifo."`
	MaxRows int64               `vfilter:"optional,field=max_rows,doc=Maximum number of rows to hold in the fifo."`
	Flush   bool                `vfilter:"optional,field=flush,doc=If specified we flush all rows from cache after the call."`
}

type _FIFOPlugin struct{}

func (self _FIFOPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "fifo", args)()

		arg := &_FIFOPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("fifo: %v", err)
			return
		}

		if arg.MaxAge == 0 {
			arg.MaxAge = 5
		}

		if arg.MaxRows == 0 {
			arg.MaxRows = 10
		}

		// Get a unique key for this query.
		key := fmt.Sprintf("fifo_%p", arg.Query)
		fifo_cache := vql_subsystem.CacheGet(scope, key)
		if fifo_cache == nil {
			scope.Log("Creating FIFO Cache for %v\n",
				vfilter.FormatToString(scope, arg.Query))
			fifo_cache = NewFIFOCache(
				ctx, scope,
				time.Duration(arg.MaxAge)*time.Second,
				arg.MaxRows,
				arg.Query)
			vql_subsystem.CacheSet(scope, key, fifo_cache)
		}

		wg.Done()

		snapshot := fifo_cache.(*_FIFOCache).Snapshot(arg.Flush)
		for _, row := range snapshot {
			select {
			case <-ctx.Done():
				return

			case output_chan <- row:
			}
		}
	}()

	// Wait until the fifo is created before returning to avoid
	// races
	wg.Wait()

	return output_chan
}

func (self _FIFOPlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "fifo",
		Doc:  "Executes 'query' and cache a number of rows from it. For each invocation we present the set of past rows.",

		ArgType: type_map.AddType(scope, &_FIFOPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_FIFOPlugin{})
}
