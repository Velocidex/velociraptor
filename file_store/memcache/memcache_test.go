package memcache

import (
	"context"
	"io/ioutil"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type MemcacheTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
}

func (self *MemcacheTestSuite) TestWriterExpiry() {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "MemcacheFileDataStore"
	config_obj.Datastore.MemcacheWriteMutationBuffer = 100
	config_obj.Datastore.MemcacheWriteMutationMaxAge = 400 // 400 Ms

	file_store := NewTestMemcacheFilestore(config_obj)

	data := []byte("Hello")

	filename := path_specs.NewSafeFilestorePath("test", "file")
	fd, err := file_store.WriteFileWithCompletion(filename, utils.SyncCompleter)
	assert.NoError(self.T(), err)

	_, err = fd.Write(data)
	assert.NoError(self.T(), err)
	fd.Close()

	// The writer is cached.
	file_store.mu.Lock()
	assert.Equal(self.T(), 1, len(file_store.data_cache))
	file_store.mu.Unlock()

	file_store.FlushCycle(context.Background())

	// Still there.
	file_store.mu.Lock()
	assert.Equal(self.T(), 1, len(file_store.data_cache))
	file_store.mu.Unlock()

	time.Sleep(400 * time.Millisecond)

	file_store.FlushCycle(context.Background())

	// Old writers are cleared after max_age
	file_store.mu.Lock()
	assert.Equal(self.T(), 0, len(file_store.data_cache))
	file_store.mu.Unlock()
}

// Size reporting is very important to keep track of the result set
// indexes.
func (self *MemcacheTestSuite) TestSizeReporting() {
	file_store := NewTestMemcacheFilestore(self.config_obj)

	data := []byte("Hello")

	filename := path_specs.NewSafeFilestorePath("test", "file")
	fd, err := file_store.WriteFileWithCompletion(filename, utils.SyncCompleter)
	assert.NoError(self.T(), err)

	_, err = fd.Write(data)
	assert.NoError(self.T(), err)

	size, err := fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(5), size)

	fd.Close()

	// Flush the filestore so it hits the disk
	file_store.Flush()

	size, err = fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(5), size)

	// Open it again with a new filestore.
	file_store = NewTestMemcacheFilestore(self.config_obj)
	fd, err = file_store.WriteFile(filename)
	assert.NoError(self.T(), err)

	_, err = fd.Write(data)
	assert.NoError(self.T(), err)

	// Size reported includes unflushed data including previously
	// written data.
	size, err = fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(10), size)

	// Read from backing store
	read_fd, err := file_store.ReadFile(filename)
	assert.NoError(self.T(), err)

	// Second write not flushed yet only has 5 bytes
	out, err := ioutil.ReadAll(read_fd)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 5, len(out))

	// Truncating the file should set its size to 0
	fd.Truncate()

	size, err = fd.Size()
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(0), size)
}

func (self *MemcacheTestSuite) TestFileAsyncWrite() {
	file_store := NewTestMemcacheFilestore(self.config_obj)

	filename := path_specs.NewSafeFilestorePath("test", "async")
	fd, err := file_store.WriteFile(filename)
	assert.NoError(self.T(), err)

	// Write some data.
	data := "Some data"
	_, err = fd.Write([]byte(data))
	assert.NoError(self.T(), err)

	// Close the file.
	fd.Close()

	// Try to read it again,
	read_fd, err := file_store.ReadFile(filename)

	// Expect an error because the file did not hit the disk yet.
	assert.Error(self.T(), err)

	// Flush the filestore so it hits the disk
	file_store.Flush()

	// Make sure it is flushed now.
	read_fd, err = file_store.ReadFile(filename)
	assert.NoError(self.T(), err)

	out, err := ioutil.ReadAll(read_fd)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), string(out), data)
}

func (self *MemcacheTestSuite) TestFileSyncWrite() {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "MemcacheFileDataStore"
	config_obj.Datastore.MemcacheWriteMutationBuffer = 100
	config_obj.Datastore.MemcacheWriteMutationMinAge = 0

	// Stop automatic flushing
	config_obj.Datastore.MemcacheWriteMutationMaxAge = 40000000000

	file_store := NewTestMemcacheFilestore(config_obj)

	filename := path_specs.NewSafeFilestorePath("test", "sync")
	fd, err := file_store.WriteFileWithCompletion(filename, utils.SyncCompleter)
	assert.NoError(self.T(), err)

	// Write some data.
	data := "Some data"
	_, err = fd.Write([]byte(data))
	assert.NoError(self.T(), err)

	// Close the file.
	fd.Close()

	// Try to read it now
	read_fd, err := file_store.ReadFile(filename)

	// This should hit the disk immediately as the sync writers wait
	// for flushes.
	assert.NoError(self.T(), err)

	out, err := ioutil.ReadAll(read_fd)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), string(out), data)
}

func (self *MemcacheTestSuite) TestFileWriteCompletions() {
	var mu sync.Mutex
	result := []string{}

	filename := path_specs.NewSafeFilestorePath("test", "completions")
	file_store := NewTestMemcacheFilestore(self.config_obj)

	fd, err := file_store.WriteFileWithCompletion(
		filename, func() {
			mu.Lock()
			defer mu.Unlock()

			result = append(result, "Done")
		})
	assert.NoError(self.T(), err)

	// Write some data.
	data := "Some data"
	_, err = fd.Write([]byte(data))
	assert.NoError(self.T(), err)

	// Close the file.
	fd.Close()

	// File is not complete yet.
	mu.Lock()
	assert.Equal(self.T(), len(result), 0)
	mu.Unlock()

	// Write again to the file.
	fd, err = file_store.WriteFileWithCompletion(
		filename, func() {
			mu.Lock()
			result = append(result, "Done")
			mu.Unlock()
		})
	assert.NoError(self.T(), err)

	// Write some data.
	_, err = fd.Write([]byte(data))
	assert.NoError(self.T(), err)

	// Close the file.
	fd.Close()

	// Make sure it is flushed now.
	file_store.Flush()

	// Both completions are fired.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		return len(result) == 2
	})

	mu.Lock()
	assert.Equal(self.T(), len(result), 2)
	assert.Equal(self.T(), "Done", result[0])
	assert.Equal(self.T(), "Done", result[1])
	mu.Unlock()
}

// Check that small writes are merged into larger writes.
func (self *MemcacheTestSuite) TestDelayedWrites() {
	metric_regex := "filestore_latency__write_.+_inf"

	snapshot := vtesting.GetMetrics(self.T(), metric_regex)

	write_times := utils.Counter{}

	// Write some data.
	data := "Some data"

	// 100 Ms between flushes
	self.config_obj.Datastore.MemcacheWriteMutationMinAge = 100

	file_store := NewTestMemcacheFilestore(self.config_obj)
	filename := path_specs.NewSafeFilestorePath("test", "merged")

	// Write 10 short messages quickly.
	for i := 0; i < 10; i++ {
		fd, err := file_store.WriteFileWithCompletion(filename, func() {
			write_times.Inc()
		})
		assert.NoError(self.T(), err)

		fd.Write([]byte(data))
		assert.NoError(self.T(), err)
		fd.Close()
	}

	// Try to flush now but it wont work because it is too quick.
	file_store.FlushCycle(context.Background())

	time.Sleep(200 * time.Millisecond)
	assert.Equal(self.T(), 0, write_times.Get())

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		// Eventually after some time the FlushCycle will cause a commit.
		file_store.FlushCycle(context.Background())
		return write_times.Get() > 0
	})

	// All the callbacks were called.
	assert.Equal(self.T(), 10, write_times.Get())

	// Make sure it is flushed now.
	read_fd, err := file_store.ReadFile(filename)
	assert.NoError(self.T(), err)

	out, err := ioutil.ReadAll(read_fd)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), strings.Repeat(string(data), 10), string(out))

	// Check that writes are merged together - 10 separate writes are
	// merged to a single one.
	snapshot = vtesting.GetMetricsDifference(self.T(), metric_regex, snapshot)

	// The MemcacheFileStore was opened and written to 10 times
	total, _ := snapshot.GetInt64("filestore_latency__write_open_MemcacheFileStore_Generic_inf")
	assert.Equal(self.T(), int64(10), total)

	total, _ = snapshot.GetInt64("filestore_latency__write_MemcacheFileWriter_Generic_inf")
	assert.Equal(self.T(), int64(10), total)

	// But the underlying memory delegate was only written once.
	total, _ = snapshot.GetInt64("filestore_latency__write_MemoryWriter_Generic_inf")
	assert.Equal(self.T(), int64(1), total)

	total, _ = snapshot.GetInt64("filestore_latency__write_open_MemoryFileStore_Generic_inf")
	assert.Equal(self.T(), int64(1), total)
}

// Create a test filestore with memory backend
func NewTestMemcacheFilestore(config_obj *config_proto.Config) *MemcacheFileStore {
	fs := NewMemcacheFileStore(context.Background(), config_obj)
	fs.delegate = memory.NewMemoryFileStore(config_obj)

	return fs
}

func TestMemcacheFileStore(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "MemcacheFileDataStore"
	config_obj.Datastore.MemcacheWriteMutationBuffer = 100

	// Stop automatic flushing
	config_obj.Datastore.MemcacheWriteMutationMinAge = 100
	config_obj.Datastore.MemcacheWriteMutationMaxAge = 4000000

	// Clear the cache between runs
	suite.Run(t, &MemcacheTestSuite{
		config_obj: config_obj,
	})
}
