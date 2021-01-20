package sorter

import (
	"bufio"
	"context"
	"io/ioutil"
	"os"
	"sort"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
	vsort "www.velocidex.com/golang/vfilter/sort"
	"www.velocidex.com/golang/vfilter/types"
)

// Implements a file based merge sort algorithm. This is important to
// limit memory use with large data sets and ORDER BY queries

type MergeSorter struct {
	ChunkSize int
}

func (self MergeSorter) Sort(ctx context.Context,
	scope types.Scope,
	input <-chan types.Row,
	key string,
	desc bool) <-chan types.Row {
	sorter, _ := self.sortWithCtx(ctx, scope, input, key, desc)
	return sorter
}

// Break out for testing.
func (self MergeSorter) sortWithCtx(ctx context.Context,
	scope types.Scope,
	input <-chan types.Row,
	key string,
	desc bool) (<-chan types.Row, *MergeSorterCtx) {
	output_chan := make(chan vfilter.Row)

	sort_ctx := &MergeSorterCtx{
		memory_sorter: &vsort.DefaultSorterCtx{
			Scope:   scope,
			OrderBy: key,
			Desc:    desc,
		},
		ChunkSize: self.ChunkSize,
	}
	sort_ctx.merge_files = []provider{sort_ctx}

	go func() {
		defer close(output_chan)

		// When we exit this function we merge all out chunks.
		defer sort_ctx.Merge(ctx, output_chan)

		// Feed the context all the rows until the input is
		// exhausted.
		for {
			select {
			case <-ctx.Done():
				return

			case row, ok := <-input:
				if !ok {
					return
				}
				sort_ctx.Feed(row)
			}
		}
	}()

	return output_chan, sort_ctx
}

type MergeSorterCtx struct {
	mu sync.Mutex
	wg sync.WaitGroup

	memory_sorter *vsort.DefaultSorterCtx

	// Read all these until all the data is read.
	merge_files []provider

	// Fallback size to files
	ChunkSize int
	idx       int
}

func (self *MergeSorterCtx) AddProvider(p provider) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.merge_files = append(self.merge_files, p)
}

// Feed the current sorter until we reach the chunk size then flush it
// to a disk file.
func (self *MergeSorterCtx) Feed(row types.Row) {
	self.memory_sorter.Items = append(self.memory_sorter.Items, row)

	if len(self.memory_sorter.Items) >= self.ChunkSize {
		// Replace the embedded sorter context.
		memory_sorter := self.memory_sorter
		self.memory_sorter = &vsort.DefaultSorterCtx{
			Scope:   memory_sorter.Scope,
			OrderBy: memory_sorter.OrderBy,
			Desc:    memory_sorter.Desc,
		}

		// Do this in parallel.
		self.wg.Add(1)
		go func() {
			defer self.wg.Done()

			sort.Sort(memory_sorter)
			self.AddProvider(newDataFile(
				memory_sorter.Scope,
				memory_sorter.Items,
				memory_sorter.OrderBy))
		}()
	}
}

func (self *MergeSorterCtx) Close() {}

func (self *MergeSorterCtx) Consume() {
	self.idx++
}

// Returns the last row
func (self *MergeSorterCtx) Last() types.Row {
	if self.idx < len(self.memory_sorter.Items) {
		return self.memory_sorter.Items[self.idx]
	}
	return nil
}

func (self *MergeSorterCtx) Merge(ctx context.Context, output_chan chan types.Row) {
	// Close all the files when we are done.
	defer func() {
		for _, provider := range self.merge_files {
			provider.Close()
		}
	}()

	// Sort the in-memory chunk
	sort.Sort(self.memory_sorter)

	// Wait for all the chunks to be ready.
	self.wg.Wait()

	for {
		var smallest_value types.Any
		var smallest_row types.Any

		// Find the smaller value from all providers. Scan all
		// providers and find the minimum.
		smallest_idx := -1

		for i, mr := range self.merge_files {
			// Extract the current provider row and key's value
			row := mr.Last()
			if utils.IsNil(row) {
				continue
			}

			value, _ := self.memory_sorter.Scope.Associative(
				row, self.memory_sorter.OrderBy)
			if utils.IsNil(value) {
				continue
			}

			// smallest_value is not set yet.
			if utils.IsNil(smallest_value) {
				smallest_value = value
				smallest_row = row
				smallest_idx = i

				continue
			}

			if self.memory_sorter.Desc {
				if self.memory_sorter.Scope.Gt(value, smallest_value) {
					smallest_value = value
					smallest_row = row
					smallest_idx = i
				}

			} else {
				if self.memory_sorter.Scope.Lt(value, smallest_value) {
					smallest_value = value
					smallest_row = row
					smallest_idx = i
				}
			}
		}

		// If there is no value left, we are done.
		if utils.IsNil(smallest_row) {
			return
		}

		// Consume the value from the provider with the
		// smallers value.
		self.merge_files[smallest_idx].Consume()

		// otherwise push the value on
		select {
		case <-ctx.Done():
			return

		case output_chan <- smallest_row:
		}
	}

}

type provider interface {
	Last() types.Row
	Consume()
	Close()
}

// Maintain a list of rows on a temporary file.
type dataFile struct {
	fd        *os.File
	reader    *bufio.Reader
	lastValue *ordereddict.Dict
	scope     types.Scope
	key       string
}

func (self *dataFile) Close() {
	if self.fd != nil {
		self.fd.Close()
		os.Remove(self.fd.Name())
		self.fd = nil
		self.reader = nil
	}
}

func (self *dataFile) Last() types.Row {
	return self.lastValue
}

// Called when the current value is consumed - read the next row from
// the file and sets the current value.
func (self *dataFile) Consume() {
	if self.reader == nil {
		return
	}

	row_data, err := self.reader.ReadBytes('\n')
	if err != nil {

		// File is exhausted, close it and reset.
		self.lastValue = nil
		self.Close()
		return
	}

	item := ordereddict.NewDict()
	err = item.UnmarshalJSON(row_data)
	if err != nil {
		self.lastValue = nil
		self.Close()
		return
	}

	self.lastValue = item
}

func newDataFile(scope types.Scope, items []types.Row, key string) *dataFile {
	result := &dataFile{
		scope: scope,
		key:   key,
	}

	tmpfile, err := ioutil.TempFile("", "vql")
	if err != nil {
		scope.Log("Unable to create tempfile: %v", err)
		return result
	}

	// Serialize all the rows into the file.
	serialized, err := json.MarshalJsonl(items)
	if err != nil {
		scope.Log("Unable to serialize: %v", err)
		return result
	}
	_, err = tmpfile.Write(serialized)
	if err != nil {
		scope.Log("Unable to serialize: %v", err)
		return result
	}
	tmpfile.Close()

	// Reopen the file for reading.
	fd, err := os.Open(tmpfile.Name())
	if err != nil {
		scope.Log("Unable to open file: %v", err)
		return result
	}

	result.fd = fd
	result.reader = bufio.NewReader(fd)

	result.Consume()

	return result
}
