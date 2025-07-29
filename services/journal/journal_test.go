package journal_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type JournalTestSuite struct {
	test_utils.TestSuite
}

func (self *JournalTestSuite) SetupTest() {
	var err error

	file_store.ClearGlobalFilestore()

	os.Setenv("VELOCIRAPTOR_LITERAL_CONFIG", test_utils.SERVER_CONFIG)
	self.ConfigObj, err = new(config.Loader).
		WithEnvLiteralLoader(constants.VELOCIRAPTOR_LITERAL_CONFIG).
		WithRequiredFrontend().
		WithVerbose(true).LoadAndValidate()
	require.NoError(self.T(), err)

	dir, err := tempfile.TempDir("file_store_test")
	assert.NoError(self.T(), err)

	self.ConfigObj.Datastore.Implementation = "MemcacheFileDataStore"
	//self.ConfigObj.Datastore.Implementation = "FileBaseDataStore"
	self.ConfigObj.Datastore.FilestoreDirectory = dir
	self.ConfigObj.Datastore.Location = dir

	self.LoadArtifactsIntoConfig([]string{`
name: System.Flow.Completion
type: CLIENT_EVENT
`, `
name: System.Hunt.Participation
type: SERVER_EVENT
`})

	self.TestSuite.SetupTest()
}

func (self *JournalTestSuite) TearDownTest() {
	// clean up
	fmt.Printf("Cleaning up %v\n", self.ConfigObj.Datastore.FilestoreDirectory)
	os.RemoveAll(self.ConfigObj.Datastore.FilestoreDirectory)
}

func (self *JournalTestSuite) TestJournalWriting() {
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	clock := utils.NewMockClock(time.Time{})
	start := clock.Now()

	// Simulate a slow filesystem (70 ms per filesystem access).
	defer api.InstallClockForTests(clock, 70)()

	// Get metrics snapshot
	snapshot := vtesting.GetMetrics(self.T(), ".")

	// Write 10 rows in series
	ctx := self.Ctx
	for i := 0; i < 10; i++ {
		err = journal.PushRowsToArtifact(ctx, self.ConfigObj,
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("Foo", "Bar").
				Set("i", i),
			},
			"System.Flow.Completion", "C.1234", "")
		assert.NoError(self.T(), err)
	}

	// Force the filestore to flush the data
	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	flusher, ok := file_store_factory.(api.Flusher)
	if ok {
		flusher.Flush()
	}

	// See the filestore metrics
	metrics := vtesting.GetMetricsDifference(self.T(), ".", snapshot)

	// Total number of writes on the memcache layer
	memcache_total, _ := metrics.GetInt64(
		"filestore_latency__write_MemcacheFileWriter_Generic_inf")

	// Total number of writes on the directory layer
	directory_total, _ := metrics.GetInt64(
		"filestore_latency__write_DirectoryFileWriter_Generic_inf")

	// Memcache should be combining many of the writes into larger
	// writes.
	assert.True(self.T(), directory_total*5 < memcache_total)

	// Get the total time. It should be much less than 10 times 70ms
	// (i.e. rows are not written serially).
	total_time := api.Clock().Now().Sub(start).Seconds()
	assert.True(self.T(), 0.07*10 > total_time)
}

func (self *JournalTestSuite) TestJournalJsonlWriting() {
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	clock := utils.NewMockClock(time.Time{})
	start := clock.Now()

	// Simulate a slow filesystem (70 ms per filesystem access).
	defer api.InstallClockForTests(clock, 70)()

	// Get metrics snapshot
	snapshot := vtesting.GetMetrics(self.T(), ".")

	// Write 10 rows in series
	for i := 0; i < 10; i++ {
		err = journal.PushJsonlToArtifact(self.Ctx, self.ConfigObj,
			[]byte(fmt.Sprintf("{\"For\":%q,\"i\":%d}\n", "Bar", i)), 1,
			"System.Flow.Completion", "C.1234", "")
		assert.NoError(self.T(), err)
	}

	// Force the filestore to flush the data
	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	flusher, ok := file_store_factory.(api.Flusher)
	if ok {
		flusher.Flush()
	}

	// See the filestore metrics
	metrics := vtesting.GetMetricsDifference(self.T(), ".", snapshot)

	// Total number of writes on the memcache layer
	memcache_total, _ := metrics.GetInt64(
		"filestore_latency__write_MemcacheFileWriter_Generic_inf")

	// Total number of writes on the directory layer
	directory_total, _ := metrics.GetInt64(
		"filestore_latency__write_DirectoryFileWriter_Generic_inf")

	// Memcache should be combining many of the writes into larger
	// writes.
	assert.True(self.T(), directory_total*5 < memcache_total)

	// Get the total time. It should be much less than 10 times 70ms
	// (i.e. rows are not written serially).
	total_time := api.Clock().Now().Sub(start).Seconds()
	assert.True(self.T(), 0.07*10 > total_time)
}

func TestJournalTestSuite(t *testing.T) {
	suite.Run(t, &JournalTestSuite{})
}
