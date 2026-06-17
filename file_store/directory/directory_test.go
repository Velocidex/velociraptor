package directory_test

import (
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/tests"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type DirectoryTestSuite struct {
	*tests.FileStoreTestSuite

	config_obj *config_proto.Config
	file_store *directory.DirectoryFileStore
}

func (self *DirectoryTestSuite) SetupTest() {
	dir, err := tempfile.TempDir("file_store_test")
	assert.NoError(self.T(), err)

	self.config_obj.Datastore.FilestoreDirectory = dir
	self.config_obj.Datastore.Location = dir
}

func (self *DirectoryTestSuite) TearDownTest() {
	// clean up
	os.RemoveAll(self.config_obj.Datastore.FilestoreDirectory)
}

func (self *DirectoryTestSuite) TestMultithreadedWrites() {
	filename := path_specs.NewSafeFilestorePath("Foo", "Bar")

	write_line := func() {
		writer, err := self.file_store.WriteFile(filename)
		assert.NoError(self.T(), err)
		writer.Write([]byte("Hello "))
		time.Sleep(time.Microsecond * 100)
		writer.Write([]byte("World\n"))
		writer.Close()

		// Test calling Close multiple times - it should not matter.
		writer.Close()
		writer.Close()
	}

	write_line()

	reader, err := self.file_store.ReadFile(filename)
	assert.NoError(self.T(), err)

	wg := &sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			write_line()
		}()
	}

	wg.Wait()

	// All the data should be serialized in order.
	data, err := io.ReadAll(reader)
	assert.NoError(self.T(), err)

	// At the end of the writes the locker should have no in process
	// writes.
	assert.Equal(self.T(), 0, self.file_store.Locker.Stats().InProgress)

	goldie.Assert(self.T(), "TestMultithreadedWrites", data)
}

func TestDirectoryFileStore(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	file_store_factory := directory.NewDirectoryFileStore(config_obj)

	file_store.OverrideFilestoreImplementation(config_obj, file_store_factory)

	suite.Run(t, &DirectoryTestSuite{
		FileStoreTestSuite: tests.NewFileStoreTestSuite(config_obj, file_store_factory),
		file_store:         file_store_factory,
		config_obj:         config_obj,
	})
}
