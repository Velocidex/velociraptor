package grouper

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"
)

var (
	mu sync.Mutex
)

func generateRows(column string, num_of_bins, num_of_repeats int) []*ordereddict.Dict {
	// Insert a unique value at the start so it remains when we switch
	// to merge sort.
	result := []*ordereddict.Dict{
		ordereddict.NewDict().
			Set(column, "11").
			Set("IgnoreMe", 1).
			Set("YX", "21"),
	}

	for i := 0; i < num_of_repeats; i++ {
		for j := 0; j < num_of_bins; j++ {
			result = append(result, ordereddict.NewDict().
				Set(column, fmt.Sprintf("%v", j)).
				Set("IgnoreMe", 1).
				Set("Y"+column, i))
		}
	}

	return result
}

func runQuery(t *testing.T,
	config_obj *config_proto.Config) []vfilter.Row {
	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set("rows", generateRows("X", 10, 10)))

	scope.SetGrouper(NewMergeSortGrouperFactory(config_obj, 3))
	scope.SetLogger(log.New(os.Stderr, " ", 0))
	ctx := context.Background()

	vql, err := vfilter.Parse("SELECT X, YX, count() AS Count FROM rows GROUP BY X ORDER BY X")
	assert.NoError(t, err)

	rows := []vfilter.Row{}
	for row := range vql.Eval(ctx, scope) {
		rows = append(rows, row)
	}
	scope.Close()

	return rows
}

func TestVQLGroupBy(t *testing.T) {
	mu.Lock()
	defer mu.Unlock()

	snapshot := vtesting.GetMetrics(t, "vql_.+")
	config_obj := config.GetDefaultConfig()
	golden := ordereddict.NewDict()

	// We generate 10 sets of 10 items (100). So our bins will have
	// cardinality of 10. First test make sure the limit is set high
	// enough so the 10 bins easily fit in memory.
	config_obj.Defaults.MaxInMemoryGroupBy = 100

	rows := runQuery(t, config_obj)
	golden.Set("Group By In Memory", rows)

	snapshot = vtesting.GetMetricsDifference(t, "vql_.+", snapshot)
	count, _ := snapshot.GetInt64("vql_group_by_sort_count")
	assert.Equal(t, int64(0), count)

	// Now set the limit at 5 bins - this will force all rows after 5
	// to use the mergs sort.
	config_obj.Defaults.MaxInMemoryGroupBy = 5

	merge_group_rows := runQuery(t, config_obj)
	golden.Set("Group By Sorted", merge_group_rows)

	// Make sure the result is the same
	assert.Equal(t, len(rows), len(merge_group_rows))
	assert.Equal(t, json.MustMarshalString(rows),
		json.MustMarshalString(merge_group_rows))

	goldie.Assert(t, "TestGroupBy", json.MustMarshalIndent(golden))
}
