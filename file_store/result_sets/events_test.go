package result_sets_test

import (
	"context"
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
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TimedResultSetTestSuite struct {
	suite.Suite

	config_obj         *config_proto.Config
	file_store         api.FileStore
	client_id, flow_id string
	sm                 *services.Service
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
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
}

func (self *TimedResultSetTestSuite) TearDownTest() {
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
}

type paging_spec struct {
	start, limit int
}

var timed_result_set_tests = []struct {
	start_time, end_time uint64
	cursors              []*result_sets.Cursor
	times                []int
	pages                []paging_spec
}{
	{
		0, 1587840000, // First file starts at 1587800000
		[]*result_sets.Cursor{
			{
				Timestamp: 1587800000,
				Filename:  "/clients/C.12312/monitoring/Windows.Events.ProcessCreation/2020-04-25.json",
				TotalRows: 4,
			},
		},
		[]int{
			1587800000, 1587810000, 1587820000, 1587830000, 1587840000,
		},
		[]paging_spec{
			{0, 2},
			{2, 2},
		},
	},

	// Start and end are within the first file.
	{
		1587810000, 1587850000,
		[]*result_sets.Cursor{
			{
				Timestamp: 1587810000,
				Filename:  "/clients/C.12312/monitoring/Windows.Events.ProcessCreation/2020-04-25.json",
				TotalRows: 4,
				RowIdx:    1, // Start with first offset (skip first row).
			},
		},
		[]int{
			1587810000, 1587820000, 1587830000, 1587840000, 1587850000,
		},
		[]paging_spec{
			{0, 2},
			{2, 2},
		},
	},

	// Get range that spans first and second file.
	{
		1587850000, 1587890000,
		[]*result_sets.Cursor{
			{
				Timestamp: 1587850000,
				Filename:  "/clients/C.12312/monitoring/Windows.Events.ProcessCreation/2020-04-25.json",
				TotalRows: 1,
				RowIdx:    5,
			},
			{
				Timestamp: 1587860000,
				Filename:  "/clients/C.12312/monitoring/Windows.Events.ProcessCreation/2020-04-26.json",
				TotalRows: 3,
				RowIdx:    0,
			},
		},
		[]int{
			1587850000, 1587860000, 1587870000, 1587880000, 1587890000,
		},
		[]paging_spec{
			{0, 3}, // Page across file boundary.
			{1, 3},
		},
	},

	// Get range that spans first, second and part of third file.
	{
		1587850000, 1587970000,
		[]*result_sets.Cursor{
			{
				Timestamp: 1587850000,
				Filename:  "/clients/C.12312/monitoring/Windows.Events.ProcessCreation/2020-04-25.json",
				TotalRows: 1,
				RowIdx:    5,
			},
			{
				Timestamp: 1587859200,
				Filename:  "/clients/C.12312/monitoring/Windows.Events.ProcessCreation/2020-04-26.json",
				TotalRows: 9,
				RowIdx:    0,
			},
			{
				Timestamp: 1587950000,
				Filename:  "/clients/C.12312/monitoring/Windows.Events.ProcessCreation/2020-04-27.json",
				TotalRows: 2,
				RowIdx:    0,
			},
		},
		[]int{
			1587850000, 1587860000, 1587870000, 1587880000, 1587890000,
			1587900000, 1587910000, 1587920000, 1587930000, 1587940000,
			1587950000, 1587960000, 1587970000,
		},
		[]paging_spec{
			{1, 6},
			{6, 6},
			{3, 10},
		},
	},
}

func (self *TimedResultSetTestSuite) TestTimedResultSets() {
	now := time.Unix(1587800000, 0)
	clock := &utils.MockClock{MockNow: now}

	// Start off by writing some events on a queue.
	qm := directory.NewDirectoryQueueManager(
		self.config_obj, self.file_store).(*directory.DirectoryQueueManager)
	qm.Clock = clock

	path_manager, err := artifacts.NewArtifactPathManager(
		self.config_obj,
		self.client_id,
		self.flow_id,
		"Windows.Events.ProcessCreation")
	assert.NoError(self.T(), err)

	path_manager.Clock = clock

	// Push an event every hour for 48 hours.
	for i := int64(0); i < 50; i++ {
		// Advance the clock by 1 hour.
		clock.MockNow = time.Unix(1587800000+10000*i, 0)

		err := qm.PushEventRows(path_manager,
			[]*ordereddict.Dict{ordereddict.NewDict()})
		assert.NoError(self.T(), err)
	}

	// v := test_utils.GetMemoryFileStore(self.T(), self.config_obj)
	// utils.Debug(v)

	for _, testcase := range timed_result_set_tests {
		cursors := result_sets.GetCursors(context.Background(),
			self.file_store, path_manager, testcase.start_time,
			testcase.end_time)
		assert.Equal(self.T(), testcase.cursors, cursors)

		rs_reader, err := result_sets.NewTimedResultSetReader(context.Background(),
			self.file_store, path_manager, testcase.start_time, testcase.end_time)
		assert.NoError(self.T(), err)

		all_rows := GetAllResults(rs_reader)
		assert.Equal(self.T(), all_rows, testcase.times)

		// Read pages from the reader
		for _, spec := range testcase.pages {
			pages := GetPages(rs_reader, spec.start, spec.limit)
			assert.Equal(self.T(), all_rows[spec.start:spec.start+spec.limit], pages)
		}
	}
}

func GetPages(self result_sets.ResultSetReader, start, limit int) []int {
	result := []int{}

	self.SeekToRow(int64(start))
	count := 0
	for row := range self.Rows(context.Background()) {
		count++
		result = append(result, int(utils.GetInt64(row, "_ts")))
		if count >= limit {
			break
		}
	}
	return result
}

func GetAllResults(self result_sets.ResultSetReader) []int {
	result := []int{}
	for row := range self.Rows(context.Background()) {
		result = append(result, int(utils.GetInt64(row, "_ts")))
	}
	return result
}

func TestTimedResultSets(t *testing.T) {
	suite.Run(t, &TimedResultSetTestSuite{})
}
