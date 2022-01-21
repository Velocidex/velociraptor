package memcache

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type MemcacheTestSuite struct {
	suite.Suite

	config_obj *config_proto.Config
	file_store *MemcacheFileStore
}

func (self *MemcacheTestSuite) TestFileReadWrite() {
	filename := path_specs.NewSafeFilestorePath("test", "foo")
	fd, err := self.file_store.WriteFile(filename)
	assert.NoError(self.T(), err)

	// Write some data.
	data := "Some data"
	_, err = fd.Write([]byte(data))
	assert.NoError(self.T(), err)

	// Close the file.
	fd.Close()

	read_fd, err := self.file_store.ReadFile(filename)

	// Expect an error because the file did not hit the disk yet.
	assert.Error(self.T(), err)

	// Make sure it is flushed now.
	fd.Flush()
	read_fd, err = self.file_store.ReadFile(filename)
	assert.NoError(self.T(), err)
	out, err := ioutil.ReadAll(read_fd)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), string(out), data)
}

func (self *MemcacheTestSuite) TestFileWriteCompletions() {
	var mu sync.Mutex
	result := []string{}

	filename := path_specs.NewSafeFilestorePath("test", "foo")

	fd, err := self.file_store.WriteFileWithCompletion(
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
	fd, err = self.file_store.WriteFileWithCompletion(
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
	fd.Flush()

	// Both completions are fired.
	mu.Lock()
	assert.Equal(self.T(), len(result), 2)
	assert.Equal(self.T(), "Done", result[0])
	assert.Equal(self.T(), "Done", result[1])
	mu.Unlock()
}

func (self *MemcacheTestSuite) TestDelayedWrites() {
	var mu sync.Mutex

	// Change the values from the default to speed up the test.
	self.config_obj.Datastore.MemcacheWriteMutationMinAge = 100
	self.config_obj.Datastore.MemcacheWriteMutationMaxAge = 400

	file_store := NewMemcacheFileStore(self.config_obj)

	stepper := func(d time.Duration, filename api.FSPathSpec) []int64 {
		// Record when the file is actually written
		write_times := []int64{}

		start := time.Now()
		for i := 0; i < 10; i++ {
			// Step through 1 second at a time.
			time.Sleep(d)
			fd, err := file_store.WriteFileWithCompletion(
				filename, func() {
					mu.Lock()
					defer mu.Unlock()

					// Record when the completions ran
					write_times = append(write_times,
						time.Now().Sub(start).Milliseconds()/100)
				})
			assert.NoError(self.T(), err)

			// Write some data.
			data := "Some data"
			_, err = fd.Write([]byte(data))
			assert.NoError(self.T(), err)

			// Close the file.
			fd.Close()
		}
		file_store.Flush()

		return write_times
	}

	golden := ordereddict.NewDict()

	// min age is 100 ms so if we write every 110ms, the file store
	// will flush on each write.
	filename := path_specs.NewSafeFilestorePath("test", "file1")
	write_times := stepper(110*time.Millisecond, filename)

	// Usually [2 3 4 5 6 7 8 9 10 11]
	fmt.Printf("Writes longer than min age: %v\n", write_times)

	write_times = nil

	// Writing more frequently than min age will refresh the ttl, but
	// when it reaches max_age the file should flush anyway.
	filename = path_specs.NewSafeFilestorePath("test", "file2")
	write_times = stepper(90*time.Millisecond, filename)
	golden.Set("90ms writes", write_times)

	// Due to timing the exact numbers here will vary a bit, but we
	// mostly want to capture the fact that most numbers will be
	// different, so we visually check the numbers
	// Usually [5 5 5 5 5 5 9 9 9 9]
	fmt.Printf("Writes shorter than min age but overall longer than max age: %v\n",
		write_times)
}

func TestMemcacheFileStore(t *testing.T) {
	// Make a tempdir
	var err error
	dirname, err := ioutil.TempDir("", "datastore_test")
	assert.NoError(t, err)
	defer os.RemoveAll(dirname) // clean up

	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "MemcacheFileDataStore"
	config_obj.Datastore.MemcacheWriteMutationBuffer = 100
	config_obj.Datastore.FilestoreDirectory = dirname
	config_obj.Datastore.Location = dirname

	// Clear the cache between runs
	fs := NewMemcacheFileStore(config_obj)
	suite.Run(t, &MemcacheTestSuite{
		file_store: fs,
		config_obj: config_obj,
	})
}
