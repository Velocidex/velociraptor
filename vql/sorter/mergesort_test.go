package sorter

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter/types"
)

func TestDataFile(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	rows := []types.Row{
		ordereddict.NewDict().Set("X", 1),
		ordereddict.NewDict().Set("X", 8),
		ordereddict.NewDict().Set("X", 2),
	}

	data_file := newDataFile(scope, rows, "X")
	defer data_file.Close()

	// Wait here until its ready
	data_file.Last()

	// Check the content of the backing file.
	fd, err := os.Open(data_file.fd.Name())
	assert.NoError(t, err)

	defer fd.Close()

	serialized, err := ioutil.ReadAll(fd)
	assert.NoError(t, err)

	goldie.Assert(t, "newDataFile", serialized)

	// Check the first row from the data_file
	X, _ := data_file.Last().(*ordereddict.Dict).Get("X")
	assert.Equal(t, X, uint64(1))

	// Consume this row
	data_file.Consume()

	X, _ = data_file.Last().(*ordereddict.Dict).Get("X")
	assert.Equal(t, X, uint64(8))

	// Consume the end of file.
	data_file.Consume()
	data_file.Consume()
	data_file.Consume()

	// The Last is now nil signaling the end
	assert.Nil(t, data_file.Last())
}

func TestMergeSorter(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	// Total of 8 rows
	values := []uint64{1, 8, 2, 7, 3, 9, 12, 4}
	ctx := context.Background()

	input := make(chan types.Row)

	sorter, sort_ctx := MergeSorter{ChunkSize: 3}.sortWithCtx(ctx, scope, input, "X", false)

	// Now feed the sorter some data
	for _, value := range values {
		input <- ordereddict.NewDict().Set("X", value)
	}
	close(input)

	sort_ctx.wg.Wait()

	// Now read the data out
	res := make([]types.Row, 0)
	for row := range sorter {
		sort_ctx.mu.Lock()
		// 2 providers - in memory and 2 files.
		assert.Equal(t, len(sort_ctx.merge_files), 3)
		sort_ctx.mu.Unlock()

		res = append(res, row)
	}

	goldie.AssertJson(t, "TestMergeSorter", res)
}

func TestMergeSorterDesc(t *testing.T) {
	scope := vql_subsystem.MakeScope()
	scope.SetLogger(log.New(os.Stderr, " ", 0))

	// Total of 8 rows
	values := []uint64{1, 8, 2, 7, 3, 9, 12, 4}
	ctx := context.Background()

	input := make(chan types.Row)

	sorter, sort_ctx := MergeSorter{ChunkSize: 3}.sortWithCtx(
		ctx, scope, input, "X", true)

	// Now feed the sorter some data
	for _, value := range values {
		input <- ordereddict.NewDict().Set("X", value)
	}
	close(input)

	sort_ctx.wg.Wait()

	// Now read the data out
	res := make([]types.Row, 0)
	for row := range sorter {

		sort_ctx.mu.Lock()
		// 2 providers - in memory and 2 files.
		assert.Equal(t, len(sort_ctx.merge_files), 3)
		sort_ctx.mu.Unlock()

		res = append(res, row)
	}

	goldie.AssertJson(t, "TestMergeSorterDesc", res)
}

// NULL values sort like an empty string, which comes before any
// number. Each chunk is sorted that way, so the merge must agree or
// the NULL rows of each chunk get spliced in between the numbers.
func TestMergeSorterNullValues(t *testing.T) {
	values := []interface{}{5, nil, 3, 9, nil, 1, 7, 2, nil, 8}

	for _, desc := range []bool{false, true} {
		scope := vql_subsystem.MakeScope()
		scope.SetLogger(log.New(os.Stderr, " ", 0))

		input := make(chan types.Row)
		sorter := MergeSorter{ChunkSize: 3}.Sort(
			context.Background(), scope, input, "X", desc)

		go func() {
			defer close(input)
			for _, value := range values {
				input <- ordereddict.NewDict().Set("X", value)
			}
		}()

		// Rows read back from a chunk file are uint64 but the last
		// chunk stays in memory as int, so compare the printed values.
		var res []string
		for row := range sorter {
			x, _ := row.(*ordereddict.Dict).Get("X")
			res = append(res, fmt.Sprint(x))
		}

		if desc {
			assert.Equal(t, []string{
				"9", "8", "7", "5", "3", "2", "1",
				"<nil>", "<nil>", "<nil>"}, res)
		} else {
			assert.Equal(t, []string{
				"<nil>", "<nil>", "<nil>",
				"1", "2", "3", "5", "7", "8", "9"}, res)
		}
	}
}
