package timed_test

import (
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

// Test migration from an old index arrangement.
func (self *TimedResultSetTestSuite) TestTimedResultSetMigration() {
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

	// Recreate events from older version. Previously we used the
	// regular ResultSetWriter to write unindexed files.
	file_store_factory := file_store.GetFileStore(self.ConfigObj)

	// Push an event every hour for 48 hours.
	for i := int64(0); i < 50; i++ {
		// Advance the clock by 1 hour.
		now := 1587800000 + 10000*i
		clock.Set(time.Unix(now, 0).UTC())

		writer, err := result_sets.NewResultSetWriter(
			file_store_factory, path_manager.Path(),
			nil, utils.SyncCompleter, result_sets.AppendMode)
		assert.NoError(self.T(), err)

		writer.Write(ordereddict.NewDict().
			Set("_ts", now).
			Set("Time", clock.Now()))
		writer.Close()
	}

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

	goldie.Assert(self.T(), "TestTimedResultSetMigration",
		json.MustMarshalIndent(result))
}
