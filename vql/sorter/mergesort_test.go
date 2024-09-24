package sorter

import (
	"context"
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
	defer fd.Close()
	assert.NoError(t, err)

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
