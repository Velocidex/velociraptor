package timed_test

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
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
	test_utils.TestSuite
	client_id, flow_id string
}

func (self *TimedResultSetTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()

	self.LoadArtifactsIntoConfig([]string{`
name: Windows.Events.ProcessCreation
type: CLIENT_EVENT
`})
	self.TestSuite.SetupTest()

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
}

func (self *TimedResultSetTestSuite) TestTimedResultSetWriting() {
	var mu sync.Mutex
	completion_result := []string{}

	now := time.Unix(1587800000, 0)
	clock := utils.NewMockClock(now)
	closer := utils.MockTime(clock)
	defer closer()

	// Start off by writing some events on a queue.
	path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj,
		self.client_id,
		self.flow_id,
		"Windows.Events.ProcessCreation")
	assert.NoError(self.T(), err)

	writer, err := timed.NewTimedResultSetWriter(
		self.ConfigObj, path_manager, nil, func() {
			mu.Lock()
			completion_result = append(completion_result, "Done")
			mu.Unlock()
		})
	assert.NoError(self.T(), err)

	// Push an event every hour for 48 hours.
	for i := int64(0); i < 50; i++ {
		// Advance the clock by 1 hour.
		now := 1587800000 + 10000*i
		clock.Set(time.Unix(now, 0).UTC())

		writer.Write(ordereddict.NewDict().
			Set("Time", clock.Now()).
			Set("Now", now))

		// Force the writer to flush to disk - next write will open
		// the file and append data to the end.
		writer.Flush()
	}

	// Completion does not run until we close the writer.
	assert.Equal(self.T(), 0, len(completion_result))
	writer.Close()

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return 1 == len(completion_result)
	})

	assert.Equal(self.T(), "Done", completion_result[0])

	result := ordereddict.NewDict()

	rs_reader, err := result_sets.NewTimedResultSetReader(
		self.Sm.Ctx, self.ConfigObj, path_manager)
	assert.NoError(self.T(), err)

	result.Set("Available Files", rs_reader.GetAvailableFiles(self.Sm.Ctx))

	for _, testcase := range timed_result_set_tests {
		err = rs_reader.SeekToTime(time.Unix(int64(testcase.start_time), 0))
		assert.NoError(self.T(), err)

		rs_reader.SetMaxTime(time.Unix(int64(testcase.end_time), 0))

		rows := make([]*ordereddict.Dict, 0)
		for row := range rs_reader.Rows(self.Sm.Ctx) {
			rows = append(rows, row)
		}
		result.Set(testcase.name, rows)
	}

	goldie.Assert(self.T(), "TestTimedResultSetWriting",
		json.MustMarshalIndent(result))
}

func (self *TimedResultSetTestSuite) TestTimedResultSetWritingJsonl() {
	var mu sync.Mutex
	completion_result := []string{}

	now := time.Unix(1587800000, 0)
	clock := utils.NewMockClock(now)
	closer := utils.MockTime(clock)
	defer closer()

	// Start off by writing some events on a queue.
	path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj,
		self.client_id,
		self.flow_id,
		"Windows.Events.ProcessCreation")
	assert.NoError(self.T(), err)

	writer, err := timed.NewTimedResultSetWriter(
		self.ConfigObj, path_manager, nil, func() {
			mu.Lock()
			completion_result = append(completion_result, "Done")
			mu.Unlock()
		})
	assert.NoError(self.T(), err)

	// Push an event every hour for 48 hours.
	for i := int64(0); i < 50; i++ {
		// Advance the clock by 1 hour.
		now := 1587800000 + 10000*i
		clock.Set(time.Unix(now, 0).UTC())

		// For performance critical sections it is sometimes easier to
		// build the jsonl by hand.
		writer.WriteJSONL([]byte(
			fmt.Sprintf("{\"Time\":%q,\"Now\":%d}\n",
				clock.Now().UTC().Format(time.RFC3339), now)), 1)

		// Force the writer to flush to disk - next write will open
		// the file and append data to the end.
		writer.Flush()
	}

	// Completion does not run until we close the writer.
	assert.Equal(self.T(), 0, len(completion_result))
	writer.Close()

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return 1 == len(completion_result)
	})

	assert.Equal(self.T(), "Done", completion_result[0])

	result := ordereddict.NewDict()

	rs_reader, err := result_sets.NewTimedResultSetReader(
		self.Sm.Ctx, self.ConfigObj, path_manager)
	assert.NoError(self.T(), err)

	result.Set("Available Files", rs_reader.GetAvailableFiles(self.Sm.Ctx))

	for _, testcase := range timed_result_set_tests {
		err = rs_reader.SeekToTime(time.Unix(int64(testcase.start_time), 0))
		assert.NoError(self.T(), err)

		rs_reader.SetMaxTime(time.Unix(int64(testcase.end_time), 0))

		rows := make([]*ordereddict.Dict, 0)
		for row := range rs_reader.Rows(self.Sm.Ctx) {
			rows = append(rows, row)
		}
		result.Set(testcase.name, rows)
	}

	goldie.Assert(self.T(), "TestTimedResultSetWriting",
		json.MustMarshalIndent(result))
}

func (self *TimedResultSetTestSuite) TestTimedResultSetWritingNoFlushing() {
	var mu sync.Mutex
	completion_result := []string{}

	now := time.Unix(1587800000, 0)
	clock := utils.NewMockClock(now)
	closer := utils.MockTime(clock)
	defer closer()

	// Start off by writing some events on a queue.
	path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj,
		self.client_id,
		self.flow_id,
		"Windows.Events.ProcessCreation")
	assert.NoError(self.T(), err)

	writer, err := timed.NewTimedResultSetWriter(
		self.ConfigObj, path_manager, nil, func() {
			mu.Lock()
			completion_result = append(completion_result, "Done")
			mu.Unlock()
		})
	assert.NoError(self.T(), err)

	// Push an event every hour for 48 hours.
	for i := int64(0); i < 50; i++ {
		// Advance the clock by 1 hour.
		now := 1587800000 + 10000*i
		clock.Set(time.Unix(now, 0).UTC())

		writer.Write(ordereddict.NewDict().
			Set("Time", clock.Now()).
			Set("Now", now))
	}

	// Completion does not run until we close the writer.
	assert.Equal(self.T(), 0, len(completion_result))
	writer.Close()

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return 1 == len(completion_result)
	})

	assert.Equal(self.T(), "Done", completion_result[0])
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
	self.dir, err = tempfile.TempDir("file_store_test")
	assert.NoError(self.T(), err)

	self.ConfigObj = self.LoadConfig()
	//self.ConfigObj.Datastore.Implementation = "FileBaseDataStore"
	//self.ConfigObj.Datastore.FilestoreDirectory = self.dir
	//self.ConfigObj.Datastore.Location = self.dir

	self.TimedResultSetTestSuite.SetupTest()
}

func (self *TimedResultSetTestSuiteFileBased) TearDownTest() {
	self.TimedResultSetTestSuite.TearDownTest()
	os.RemoveAll(self.dir)
}

func TestTimedResultSetWriterFileBased(t *testing.T) {
	suite.Run(t, &TimedResultSetTestSuiteFileBased{
		TimedResultSetTestSuite: TimedResultSetTestSuite{},
	})
}
