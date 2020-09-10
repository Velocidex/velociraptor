package result_sets_test

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
)

type path_tests_t struct {
	client_id, flow_id, full_artifact_name string
	expected                               string
}

var path_tests = []path_tests_t{
	// Regular client artifact
	{"C.123", "F.123", "Windows.Sys.Users",
		"/clients/C.123/artifacts/Windows.Sys.Users/F.123.json"},

	// Artifact with source
	{"C.123", "F.123", "Generic.Client.Info/Users",
		"/clients/C.123/artifacts/Generic.Client.Info/F.123/Users.json"},

	// Server artifacts
	{"C.123", "F.123", "Server.Utils.CreateCollector",
		"/clients/server/artifacts/Server.Utils.CreateCollector/F.123.json"},

	// Server events
	{"C.123", "F.123", "Elastic.Flows.Upload",
		"/server_artifacts/Elastic.Flows.Upload/2020-04-25.json"},

	// Client events
	{"C.123", "F.123", "Windows.Events.ProcessCreation",
		"/clients/C.123/monitoring/Windows.Events.ProcessCreation/2020-04-25.json"},
}

type PathManageTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	sm         *services.Service
	dirname    string
}

func (self *PathManageTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.dirname, err := ioutil.TempDir("", "path_manager_test")
	assert.NoError(self.T(), err)

	self.config_obj.Datastore.Implementation = "FileBaseDataStore"
	self.config_obj.Datastore.FilestoreDirectory = dir
	self.config_obj.Datastore.Location = dir

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)
	assert.NotNil(self.T(), self.sm)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
}

func (self *PathManageTestSuite) TearDownTest() {
	self.sm.Close()
	os.RemoveAll(self.dirname) // clean up
}

// The path manager maps artifacts, clients, flows etc into a file
// store path. For event artifacts, the path manager splits the files
// by day to ensure they are not too large and can be easily archived.
func (self *PathManageTestSuite) TestPathManager() {
	ts := int64(1587800823)

	for _, testcase := range path_tests {
		path_manager := result_sets.NewArtifactPathManager(
			self.config_obj,
			testcase.client_id,
			testcase.flow_id,
			testcase.full_artifact_name)
		path_manager.Clock = utils.MockClock{MockNow: time.Unix(ts, 0)}
		path, err := path_manager.GetPathForWriting()
		assert.NoError(self.T(), err)
		assert.Equal(self.T(), path, testcase.expected)

		file_store := memory.Test_memory_file_store
		file_store.Clear()

		qm := memory.NewMemoryQueueManager(
			self.config_obj, file_store).(*memory.MemoryQueueManager)
		qm.Clock = path_manager.Clock

		qm.PushEventRows(path_manager,
			[]*ordereddict.Dict{ordereddict.NewDict()})

		data, ok := file_store.Get(testcase.expected)
		assert.Equal(self.T(), ok, true)
		assert.Equal(self.T(), string(data), "{\"_ts\":1587800823}\n")
	}
}

// Test the path manager with DirectoryFileStore
func (self *PathManageTestSuite) TestPathManagerDailyRotations() {
	t := self.T()

	file_store_factory := file_store.GetFileStore(self.config_obj)
	clock := &utils.MockClock{}

	path_manager := result_sets.NewArtifactPathManager(
		self.config_obj,
		"C.123",
		"F.123",
		"Windows.Events.ProcessCreation")
	path_manager.Clock = clock

	qm := directory.NewDirectoryQueueManager(
		self.config_obj,
		file_store_factory).(*directory.DirectoryQueueManager)
	qm.Clock = clock

	// Write 3 different events in different days
	timestamps := []int64{1587200823, 1587300823, 1587400823}
	for _, ts := range timestamps {
		clock.MockNow = time.Unix(ts, 0)
		qm.PushEventRows(path_manager,
			[]*ordereddict.Dict{ordereddict.NewDict()})
	}

	ctx := context.Background()
	results := []*api.ResultSetFileProperties{}
	for child := range path_manager.GeneratePaths(ctx) {
		results = append(results, child)
	}

	assert.Equal(t, len(results), 3)
	for idx, result := range results {
		assert.True(t, timestamps[idx] > result.StartTime,
			"Timestamp %v %v", timestamps[idx], result)
		assert.True(t, timestamps[idx] < result.EndTime,
			"Timestamp %v %v", timestamps[idx], result)
	}

	// Test GetTimeRange - no time range specified should return
	// all items.
	times := []int64{}
	row_chan, err := file_store.GetTimeRange(ctx, self.config_obj,
		path_manager, 0, 0)
	assert.NoError(t, err)
	for row := range row_chan {
		ts, _ := row.Get("_ts")
		times = append(times, ts.(int64))
	}
	assert.Equal(t, times, timestamps)

	// Cover a small time range - no end time
	times = nil
	row_chan, err = file_store.GetTimeRange(ctx, self.config_obj,
		path_manager, 1587300822, 0)
	assert.NoError(t, err)
	for row := range row_chan {
		ts, _ := row.Get("_ts")
		times = append(times, ts.(int64))
	}
	assert.Equal(t, times, timestamps[1:])

	// Cover a small time range - no start time
	times = nil
	row_chan, err = file_store.GetTimeRange(ctx, self.config_obj,
		path_manager, 0, 1587300824)
	assert.NoError(t, err)
	for row := range row_chan {
		ts, _ := row.Get("_ts")
		times = append(times, ts.(int64))
	}
	assert.Equal(t, times, timestamps[:2])
}

func TestPathTest(t *testing.T) {
	suite.Run(t, &PathManageTestSuite{})
}
