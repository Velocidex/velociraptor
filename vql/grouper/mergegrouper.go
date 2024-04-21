package grouper

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/aggregators"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	groupByMergeSortCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vql_group_by_sort_count",
		Help: "How many rows were sorted as part of the group by strategy.",
	})
)

const (
	GROUPBY_COLUMN = "$"
)

// Aggregate functions (count, sum etc)
// operate by storing data in the scope
// context between rows. When we group by we
// create a different scope context for each
// bin - all the rows with the same group by
// value are placed in the same bin and share
// the same context.
type AggregateContext struct {
	// The last row evaluated - this is the row we will emit at the end.
	row *ordereddict.Dict

	// The context for evaluating the row.
	context types.AggregatorCtx
}

/*
This is a memory efficient grouper with a contrained upper bound on
memory consumption.

 1. Grouped by rows are grouped into bins with a constant group key
 2. When the number of bins exceeds the chunk size, we:
 3. Sort the bins by the group key and then serialized the bins into a tmp file.
 4. When the query is finished we perform a merge-sort on the resulting files:
 5. Reading the bins from various files by order of the group key, we
    can group duplicate bins from each file.
*/
type MergeSortGrouper struct {
	ChunkSize  int
	config_obj *config_proto.Config

	bins *ordereddict.Dict // map[string]*AggregateContext
}

func (self *MergeSortGrouper) getContext(key string) *AggregateContext {
	aggregate_ctx, pres := self.bins.Get(key)
	if pres && !utils.IsNil(aggregate_ctx) {
		return aggregate_ctx.(*AggregateContext)
	}

	new_aggregate_ctx := &AggregateContext{
		context: aggregators.NewAggregatorCtx(),
	}
	self.bins.Set(key, new_aggregate_ctx)
	return new_aggregate_ctx
}

func (self *MergeSortGrouper) flushContext(
	ctx context.Context, output_chan chan vfilter.Row,
	key string, aggregate_ctx *AggregateContext) {
	if aggregate_ctx != nil {
		// Send the old row from the old context - we wont need it
		// again.
		select {
		case <-ctx.Done():
			return

		case output_chan <- aggregate_ctx.row:
		}
		self.bins.Delete(key)
	}
}

func (self *MergeSortGrouper) groupWithSorting(
	ctx context.Context, scope types.Scope,
	output_chan chan types.Row,
	actor types.GroupbyActor) {

	group_sorter := &sorter.MergeSorter{ChunkSize: self.ChunkSize}
	row_chan := make(chan types.Row)

	sorted_rows := group_sorter.Sort(ctx, scope, row_chan, GROUPBY_COLUMN, false)

	// Feed the sorter data from the group by clause
	go func() {
		defer close(row_chan)

		for {
			_, row, bin_idx, new_scope, err := actor.GetNextRow(ctx, scope)
			if err != nil {
				break
			}

			materialized_row := actor.MaterializeRow(ctx, row, new_scope).
				Set(GROUPBY_COLUMN, bin_idx)

			new_scope.ChargeOp()

			select {
			case <-ctx.Done():
				new_scope.Close()
				return

			case row_chan <- materialized_row:
			}

			groupByMergeSortCount.Inc()
			new_scope.Close()
		}
	}()

	// Now read the sorted rows and transform them again to get
	// aggregate functions to apply to the data.
	last_gb_element := ""
	var aggregate_ctx *AggregateContext

	for sorted_row := range sorted_rows {
		materialized_row := actor.MaterializeRow(ctx, sorted_row, scope)
		gb_element, pres := materialized_row.GetString(GROUPBY_COLUMN)
		if !pres {
			continue
		}
		materialized_row.Delete(GROUPBY_COLUMN)

		// The new gb element is different from the last one, flush
		// the row and start a new context.
		if gb_element != last_gb_element {
			self.flushContext(ctx, output_chan, last_gb_element, aggregate_ctx)

			// Start processing the new gb element
			last_gb_element = gb_element

			// Check if the aggregate context was seen previously
			aggregate_ctx = self.getContext(gb_element)
		}

		// Evaluate the row with the current bin context and keep the
		// result for next time.
		aggregate_ctx.row = self.transformRow(
			ctx, scope, aggregate_ctx.context, actor, materialized_row)
	}

	self.flushContext(ctx, output_chan, last_gb_element, aggregate_ctx)
	self.emitBins(ctx, output_chan)
}

func (self *MergeSortGrouper) transformRow(
	ctx context.Context, scope types.Scope,
	context types.AggregatorCtx,
	actor types.GroupbyActor, row *ordereddict.Dict) *ordereddict.Dict {

	// Create a new scope over which we can evaluate the filter
	// clause.
	new_scope := scope.Copy()
	defer new_scope.Close()

	transformed_row, closer := actor.Transform(ctx, new_scope, row)
	defer closer()

	// Order matters - transformed row (from column specifiers) may
	// mask original row (from plugin).
	new_scope.AppendVars(row)
	new_scope.AppendVars(transformed_row)
	new_scope.SetAggregatorCtx(context)

	return actor.MaterializeRow(ctx, transformed_row, new_scope)
}

func (self *MergeSortGrouper) Group(
	ctx context.Context, scope types.Scope, actor types.GroupbyActor) <-chan types.Row {
	output_chan := make(chan types.Row)

	max_in_memory_group_by := uint64(30000)
	if self.config_obj != nil &&
		self.config_obj.Defaults != nil &&
		self.config_obj.Defaults.MaxInMemoryGroupBy > 0 {
		max_in_memory_group_by = self.config_obj.Defaults.MaxInMemoryGroupBy
	}

	go func() {
		defer close(output_chan)

		// Append this row to a bin based on a unique value of the
		// group by column.
		for {
			transformed_row, _, bin_idx, new_scope, err := actor.GetNextRow(
				ctx, scope)
			if err != nil {
				break
			}

			// Try to find the context in the map
			aggregate_ctx := self.getContext(bin_idx)

			// The transform function receives its own unique context
			// for the specific aggregate group.
			new_scope.SetAggregatorCtx(aggregate_ctx.context)

			// Update the row with the transformed columns. Note we
			// must materialize these rows because evaluating the row
			// may have side effects (e.g. for aggregate functions).
			new_row := actor.MaterializeRow(ctx, transformed_row, new_scope)

			aggregate_ctx.row = new_row

			// Bins are too large we switch to the slower sort method
			// which is memory constrained.
			if self.bins.Len() > int(max_in_memory_group_by) {
				scope.Log("GROUP BY: %v bins exceeded, Switching to slower file based operation",
					self.bins.Len())
				self.groupWithSorting(ctx, new_scope, output_chan, actor)
				new_scope.Close()
				return
			}
			new_scope.Close()
		}

		self.emitBins(ctx, output_chan)
	}()

	return output_chan
}

// Flush all the rows in the current bin set.
func (self *MergeSortGrouper) emitBins(
	ctx context.Context, output_chan chan vfilter.Row) {

	for _, key := range self.bins.Keys() {
		aggregate_ctx := self.getContext(key)
		select {
		case <-ctx.Done():
			return

		case output_chan <- aggregate_ctx.row:
		}
	}
}

type MergeSortGrouperFactory struct {
	config_obj *config_proto.Config
	ChunkSize  int
}

func NewMergeSortGrouperFactory(
	config_obj *config_proto.Config, chunk_size int) types.Grouper {
	return MergeSortGrouperFactory{
		config_obj: config_obj,
		ChunkSize:  chunk_size,
	}
}

func (self MergeSortGrouperFactory) Group(
	ctx context.Context, scope types.Scope, actor types.GroupbyActor) <-chan types.Row {
	grouper := &MergeSortGrouper{
		config_obj: self.config_obj,
		ChunkSize:  self.ChunkSize,

		// Use an ordereddict here to maintain stable row ordering.
		bins: ordereddict.NewDict(),
	}
	return grouper.Group(ctx, scope, actor)
}
