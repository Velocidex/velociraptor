package timed

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
)

// We write files in the following ranges:
// 1. 1587800000-1587850000
// 2. 1587860000-1587940000
// 3. 1587950000-1588030000
var timed_result_set_tests = []struct {
	name                 string
	start_time, end_time uint64
}{
	{
		"Start in second file end in second file", 1587863000, 1587900000,
	},
	{
		"From 0 to midway through first file", 0, 1587840000,
	},
	{
		"Start and end are within the first file.", 1587810000, 1587850000,
	},
	{
		"Get range that spans first and second file.", 1587850000, 1587890000,
	},
	{
		"Get range that spans first, second and part of third file.",
		1587850000, 1587970000,
	},
	{
		"Exceed available time range from last file.", 1588270000, 1887970000,
	},
	{
		"Exceed available time range.", 1788270000, 1887970000,
	},
}

type TimedResultSetTestSuite struct {
	suite.Suite

	config_obj         *config_proto.Config
	file_store         api.FileStore
	client_id, flow_id string
	sm                 *services.Service
	ctx                context.Context
}

func (self *TimedResultSetTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
	self.file_store = file_store.GetFileStore(self.config_obj)

	// Start essential services.
	self.ctx, _ = context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(self.ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
}

func (self *TimedResultSetTestSuite) TearDownTest() {
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
}

func (self *TimedResultSetTestSuite) TestTimedResultSetWriting() {
	now := time.Unix(1587800000, 0)
	clock := &utils.MockClock{MockNow: now}

	// Start off by writing some events on a queue.
	path_manager, err := artifacts.NewArtifactPathManager(
		self.config_obj,
		self.client_id,
		self.flow_id,
		"Windows.Events.ProcessCreation")
	assert.NoError(self.T(), err)
	path_manager.Clock = clock

	file_store_factory := file_store.GetFileStore(self.config_obj)
	writer, err := NewTimedResultSetWriter(
		file_store_factory, path_manager, nil)
	assert.NoError(self.T(), err)

	writer.(*TimedResultSetWriterImpl).Clock = clock

	// Push an event every hour for 48 hours.
	for i := int64(0); i < 50; i++ {
		// Advance the clock by 1 hour.
		now := 1587800000 + 10000*i
		clock.MockNow = time.Unix(now, 0)

		writer.Write(ordereddict.NewDict().
			Set("Time", clock.MockNow).
			Set("Now", now))
		writer.Flush()
	}

	writer.Close()

	// test_utils.GetMemoryFileStore(self.T(), self.config_obj).Debug()

	result := ordereddict.NewDict()

	rs_reader, err := result_sets.NewTimedResultSetReader(
		self.ctx, self.file_store, path_manager)
	assert.NoError(self.T(), err)

	result.Set("Available Files", rs_reader.GetAvailableFiles(self.ctx))

	for _, testcase := range timed_result_set_tests {
		err = rs_reader.SeekToTime(time.Unix(int64(testcase.start_time), 0))
		assert.NoError(self.T(), err)

		rs_reader.SetMaxTime(time.Unix(int64(testcase.end_time), 0))

		rows := make([]*ordereddict.Dict, 0)
		for row := range rs_reader.Rows(self.ctx) {
			rows = append(rows, row)
		}
		result.Set(testcase.name, rows)
	}

	goldie.Assert(self.T(), "TestTimedResultSetWriting",
		json.MustMarshalIndent(result))
}

func TestTimedResultSets(t *testing.T) {
	suite.Run(t, &TimedResultSetTestSuite{})
}

type TimedResultSetTestSuiteFileBased struct {
	TimedResultSetTestSuite
	dir string
}

func (self *TimedResultSetTestSuiteFileBased) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.dir, err = ioutil.TempDir("", "file_store_test")
	assert.NoError(self.T(), err)

	self.ctx, _ = context.WithTimeout(context.Background(), time.Second*60)
	self.config_obj.Datastore.Implementation = "FileBaseDataStore"
	self.config_obj.Datastore.FilestoreDirectory = self.dir
	self.config_obj.Datastore.Location = self.dir

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
	self.file_store = file_store.GetFileStore(self.config_obj)
}

func (self *TimedResultSetTestSuiteFileBased) TearDownTest() {
	os.RemoveAll(self.dir)
}

func TestTimedResultSetWriterFileBased(t *testing.T) {
	suite.Run(t, &TimedResultSetTestSuiteFileBased{
		TimedResultSetTestSuite: TimedResultSetTestSuite{},
	})
}
