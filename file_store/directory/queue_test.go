package directory_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/file_store/tests"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

var (
	monitoringArtifact = `
name: TestQueue
type: SERVER_EVENT
`
)

func TestDirectoryQueueManager(t *testing.T) {
	dir, err := tempfile.TempDir("file_store_test")
	assert.NoError(t, err)

	defer os.RemoveAll(dir) // clean up

	ConfigObj := config.GetDefaultConfig()
	ConfigObj.Datastore.Implementation = "FileBaseDataStore"
	ConfigObj.Datastore.FilestoreDirectory = dir
	ConfigObj.Datastore.Location = dir

	file_store_factory := memory.NewMemoryFileStore(ConfigObj)
	manager := directory.NewDirectoryQueueManager(ConfigObj, file_store_factory)

	file_store.OverrideFilestoreImplementation(ConfigObj, file_store_factory)

	suite.Run(t, tests.NewQueueManagerTestSuite(
		ConfigObj, manager, file_store_factory))
}

type TestSuite struct {
	test_utils.TestSuite
	client_id string
	dir       string
}

func (self *TestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	dir, err := tempfile.TempDir("file_store_test")
	assert.NoError(self.T(), err)
	self.dir = dir

	os.Setenv("temp", dir)
	self.client_id = "C.12312"
}

func (self *TestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	os.RemoveAll(self.dir) // clean up
}

func (self *TestSuite) TestQueueManager() {
	repo_manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := repo_manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(monitoringArtifact,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})
	assert.NoError(self.T(), err)

	file_store := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	manager := directory.NewDirectoryQueueManager(
		self.ConfigObj, file_store).(*directory.DirectoryQueueManager)

	// Push some rows to the queue manager
	ctx := context.Background()

	reader, cancel := manager.Watch(ctx, "TestQueue", &api.QueueOptions{
		FileBufferLeaseSize: 1,
	})

	path_manager, err := artifacts.NewArtifactPathManager(self.Ctx, self.ConfigObj,
		"C.123", "", "TestQueue")
	assert.NoError(self.T(), err)

	// Query the state of the manager for testing.
	dbg := manager.Debug()
	// The initial size is zero
	assert.Equal(self.T(), int64(0), utils.GetInt64(dbg, "TestQueue.0.Size"))

	// Push some rows without reading - this should write to the
	// file buffer and not block.
	for i := 0; i < 10; i++ {
		err = manager.PushEventRows(path_manager, []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("Foo", "Bar"),
		})
		assert.NoError(self.T(), err)
	}

	vtesting.WaitUntil(15*time.Second, self.T(), func() bool {
		// The file should contain all the rows now.  File size is not
		// exact due to timestamps but it should be larger than 300.
		dbg = manager.Debug()
		return utils.GetInt64(dbg, "TestQueue.0.Size") > int64(300) &&
			utils.GetString(dbg, "TestQueue.0.BackingFile") != ""
	})

	dbg = manager.Debug()
	backing_file := utils.GetString(dbg, "TestQueue.0.BackingFile")

	// Now read 10 rows from the file.
	count := 0
	for row := range reader {
		count++
		assert.Equal(self.T(), "Bar", utils.GetString(row, "Foo"))

		// Break on the 10th row
		if count >= 10 {
			break
		}
	}

	// Now check the file - it should be truncated since we read all
	// messages. This will also clear the tempfile.
	dbg = manager.Debug()
	assert.Equal(self.T(), "", utils.GetString(dbg, "TestQueue.0.BackingFile"))

	// Now cancel the watcher - further reads from the channel
	// should not block - the channel is closed.
	cancel()

	for range reader {
	}

	// Now make sure the tempfile is removed.
	_, err = os.Stat(backing_file)
	assert.Error(self.T(), err)
}

func (self *TestSuite) TestQueueManagerJsonl() {
	repo_manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := repo_manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(monitoringArtifact,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)

	file_store := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	manager := directory.NewDirectoryQueueManager(
		self.ConfigObj, file_store).(*directory.DirectoryQueueManager)

	path_manager, err := artifacts.NewArtifactPathManager(self.Ctx, self.ConfigObj,
		"C.123", "", "TestQueue")
	assert.NoError(self.T(), err)

	// Query the state of the manager for testing.
	dbg := manager.Debug()

	// The initial size is zero
	assert.Equal(self.T(), int64(0), utils.GetInt64(dbg, "TestQueue.0.Size"))

	// Push some rows without reading - this should write to the
	// file buffer and not block.
	for i := 0; i < 10; i++ {
		// For performance critical parts it is more efficient to
		// build the JSONL manually
		err = manager.PushEventJsonl(path_manager,
			[]byte(fmt.Sprintf("{\"Foo\":%q}\n", "Bar")), 1)
		assert.NoError(self.T(), err)
	}

	vtesting.WaitUntil(15*time.Second, self.T(), func() bool {
		// The file should have 10 records or 11 lines.
		return len(strings.Split(test_utils.FileReadAll(
			self.T(), self.ConfigObj, path_manager.Path()), "\n")) == 11
	})
}

func TestFileBasedQueueManager(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
