package sorter

import (
	"bufio"
	"context"
	"os"
	"sort"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	utils_tempfile "www.velocidex.com/golang/velociraptor/utils/tempfile"
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

	go func() {
		defer close(output_chan)

		// When we exit this function we merge all our chunks.
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
}

func (self *MergeSorterCtx) addProvider(p provider) {
	self.merge_files = append(self.merge_files, p)
}

// Feed the current sorter until we reach the chunk size then flush it
// to a disk file.
func (self *MergeSorterCtx) Feed(row types.Row) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.memory_sorter.Items = append(self.memory_sorter.Items, row)

	if len(self.memory_sorter.Items) >= self.ChunkSize {
		// Replace the embedded sorter context.
		memory_sorter := self.memory_sorter
		sort.Sort(memory_sorter)

		self.memory_sorter = &vsort.DefaultSorterCtx{
			Scope:   memory_sorter.Scope,
			OrderBy: memory_sorter.OrderBy,
			Desc:    memory_sorter.Desc,
		}

		new_data_file := newDataFile(
			memory_sorter.Scope,
			memory_sorter.Items,
			memory_sorter.OrderBy)

		self.addProvider(new_data_file)
	}
}

func (self *MergeSorterCtx) Merge(ctx context.Context, output_chan chan types.Row) {
	// Close all the files when we are done.
	defer func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		for _, provider := range self.merge_files {
			provider.Close()
		}
	}()

	// Wait for all the chunks to be ready.
	self.wg.Wait()

	// Sort the last in-memory chunk and add it as a provider.
	if len(self.memory_sorter.Items) > 0 {
		sort.Sort(self.memory_sorter)
		self.addProvider(&memoryProvider{
			Items: self.memory_sorter.Items,
		})
	}

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

			value, pres := self.memory_sorter.Scope.Associative(
				row, self.memory_sorter.OrderBy)
			if !pres {
				self.memory_sorter.Scope.Log("Order by column %v not present in row",
					self.memory_sorter.OrderBy)
			}

			// Treat NULL as a string so they sort properly.
			if utils.IsNil(value) {
				value = ""
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
	mu sync.Mutex

	fd        *os.File
	reader    *bufio.Reader
	lastValue *ordereddict.Dict
	scope     types.Scope
	key       string
}

func (self *dataFile) Close() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Close()
}

func (self *dataFile) _Close() {
	if self.fd != nil {
		self.fd.Close()
		err := os.Remove(self.fd.Name())
		utils_tempfile.RemoveTmpFile(self.fd.Name(), err)

		self.fd = nil
		self.reader = nil
	}
}

func (self *dataFile) Last() types.Row {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.lastValue
}

// Called when the current value is consumed - read the next row from
// the file and sets the current value.
func (self *dataFile) Consume() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self._Consume()
}

func (self *dataFile) _Consume() {
	if self.reader == nil {
		return
	}

	row_data, err := self.reader.ReadBytes('\n')
	if err != nil {
		// File is exhausted, close it and reset.
		self.lastValue = nil
		self._Close()
		return
	}

	item := ordereddict.NewDict()
	err = item.UnmarshalJSON(row_data)
	if err != nil {
		self.lastValue = nil
		self._Close()
		return
	}

	self.lastValue = item
}

// Initialize the file by writing it to storage. Writing is done in the background.
func (self *dataFile) prepareFile(scope vfilter.Scope, items []vfilter.Row) {

	// We hold the lock for the duration of writing the file until we
	// are ready.
	self.mu.Lock()
	go func() {
		defer self.mu.Unlock()

		tmpfile, err := tempfile.TempFile("vql")
		if err != nil {
			scope.Log("Unable to create tempfile: %v", err)
			return
		}

		utils_tempfile.AddTmpFile(tmpfile.Name())

		// Serialize all the rows into the file.
		serialized, err := json.MarshalJsonl(items)
		if err != nil {
			scope.Log("Unable to serialize: %v", err)
			return
		}
		_, err = tmpfile.Write(serialized)
		if err != nil {
			scope.Log("Unable to serialize: %v", err)
			return
		}
		tmpfile.Close()

		// Reopen the file for reading.
		fd, err := os.Open(tmpfile.Name())
		if err != nil {
			scope.Log("Unable to open file: %v", err)
			return
		}

		self.fd = fd
		self.reader = bufio.NewReader(fd)

		self._Consume()
	}()
}

func newDataFile(scope types.Scope, items []types.Row, key string) *dataFile {
	self := &dataFile{
		scope: scope,
		key:   key,
	}

	self.prepareFile(scope, items)
	return self
}

// A provider for in memory rows
type memoryProvider struct {
	Items []types.Row
	idx   int
}

func (self *memoryProvider) Last() types.Row {
	if self.idx < len(self.Items) {
		return self.Items[self.idx]
	}
	return nil
}

func (self *memoryProvider) Consume() {
	self.idx++
}

func (self *memoryProvider) Close() {}
