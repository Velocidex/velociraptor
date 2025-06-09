package common

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

func (self *_FIFOCache) PushWithCallback(
	row vfilter.Row, CallBack func(rows []types.Row) bool) {

	self.mu.Lock()
	defer self.mu.Unlock()

	self._Push(row)

	rows := []vfilter.Row{}
	for idx := self.rows.Front(); idx != nil; idx = idx.Next() {
		entry := idx.Value.(*_FIFOCacheEntry)
		rows = append(rows, entry.row)
	}

	// Call the callback to determine if we need to clear the cache.
	if CallBack(rows) {
		self.rows = list.New()
		self.count = 0
	}
}

type _SequencePluginArgs struct {
	Query  types.StoredQuery `vfilter:"required,field=query,doc=Run this query to generate rows. The query should select from SEQUENCE which will contain the current set of rows in the sequence. The query will be run on each new row that is pushed to the sequence."`
	MaxAge int64             `vfilter:"optional,field=max_age,doc=Maximum number of seconds to hold rows in the sequence."`
}

func getQueries(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) []types.StoredQuery {
	queries := []types.StoredQuery{}
	for _, member := range scope.GetMembers(args) {
		switch member {
		case "query", "max_age":
			continue
		}

		member_obj, pres := args.Get(member)
		if pres {
			queries = append(queries, arg_parser.ToStoredQuery(ctx, member_obj))
		}
	}
	return queries
}

func evalQuery(
	ctx context.Context,
	scope vfilter.Scope,
	query types.StoredQuery,
	wg *sync.WaitGroup, fifo *_FIFOCache,

	// Will be called after each row is added
	processRowCB func(rows []types.Row) bool) {
	defer wg.Done()

	new_scope := scope.Copy()
	defer new_scope.Close()

	for item := range query.Eval(ctx, new_scope) {
		select {
		case <-ctx.Done():
			return

		default:
			fifo.PushWithCallback(item, processRowCB)
		}
	}
}

type _SequencePlugin struct{}

func (self _SequencePlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "sequence", args)()

		query_any, pres := args.Get("query")
		if !pres {
			scope.Log("sequence: `query` parameter should specify a query for the sequence.")
			return
		}

		query, ok := query_any.(types.StoredQuery)
		if !ok {
			scope.Log("sequence: `query` parameter should specify a query for the sequence.")
			return
		}

		max_age, pres := args.GetInt64("max_age")
		if !pres || max_age == 0 {
			max_age = 30
		}

		fifo := &_FIFOCache{
			rows:     list.New(),
			max_time: time.Duration(max_age) * time.Second,
			max_rows: 1000,
		}

		wg := &sync.WaitGroup{}

		// Start all queries to feed into the fifo
		for _, subquery := range getQueries(ctx, scope, args) {
			wg.Add(1)
			go evalQuery(ctx, scope, subquery, wg, fifo,
				func(rows []types.Row) bool {
					// We are called whenever a row is added. We need to
					// run the query over the fifo and emit any rows.
					new_scope := scope.Copy()
					defer new_scope.Close()

					new_scope.AppendVars(ordereddict.NewDict().
						Set("SEQUENCE", rows))

					// If we return any rows we should clear the cache.
					clear := false
					for row := range query.Eval(ctx, new_scope) {
						select {
						case <-ctx.Done():
							return true

						case output_chan <- row:
							clear = true
						}
					}
					return clear
				})
		}

		// Wait here for all the queries to end.
		wg.Wait()
	}()

	return output_chan
}

func (self _SequencePlugin) Info(
	scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "sequence",
		Doc:  "Combines the output of many queries into an in memory fifo. After each row is received from any subquery runs the query specified in the 'query' parameter to retrieve rows from the memory SEQUENCE object.",

		ArgType: type_map.AddType(scope, &_SequencePluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&_SequencePlugin{})
}
