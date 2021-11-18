package journal_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type JournalTestSuite struct {
	test_utils.TestSuite
}

func (self *JournalTestSuite) SetupTest() {
	var err error
	os.Setenv("VELOCIRAPTOR_CONFIG", test_utils.SERVER_CONFIG)
	self.ConfigObj, err = new(config.Loader).
		WithEnvLiteralLoader("VELOCIRAPTOR_CONFIG").WithRequiredFrontend().
		WithVerbose(true).LoadAndValidate()
	require.NoError(self.T(), err)

	dir, err := ioutil.TempDir("", "file_store_test")
	assert.NoError(self.T(), err)

	self.ConfigObj.Datastore.Implementation = "MemcacheFileDataStore"
	//self.ConfigObj.Datastore.Implementation = "FileBaseDataStore"
	self.ConfigObj.Datastore.FilestoreDirectory = dir
	self.ConfigObj.Datastore.Location = dir

	self.TestSuite.SetupTest()

	self.LoadArtifacts([]string{`
name: System.Flow.Completion
type: CLIENT_EVENT
`, `
name: System.Hunt.Participation
type: SERVER_EVENT
`})
}

func (self *JournalTestSuite) TearDownTest() {
	// clean up
	fmt.Printf("Cleaning up %v\n", self.ConfigObj.Datastore.FilestoreDirectory)
	os.RemoveAll(self.ConfigObj.Datastore.FilestoreDirectory)
}

func (self *JournalTestSuite) TestJournalWriting() {
	journal, err := services.GetJournal()
	assert.NoError(self.T(), err)

	clock := &utils.MockClock{}
	start := clock.Now()

	// Simulate a slow filesystem (70 ms per filesystem access).
	defer api.InstallClockForTests(clock, 70)()

	// Get metrics snapshot
	snapshot := vtesting.GetMetrics(self.T(), ".")

	// Write 10 rows in series
	for i := 0; i < 10; i++ {
		err = journal.PushRowsToArtifact(self.ConfigObj,
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
	//	json.Dump(vtesting.GetMetricsDifference(self.T(), "^filestore_", snapshot))
	json.Dump(vtesting.GetMetricsDifference(self.T(), ".", snapshot))

	// Get the total time.
	fmt.Printf("Total time %v\n", api.Clock.Now().Sub(start).Seconds())

}

func TestJournalTestSuite(t *testing.T) {
	suite.Run(t, &JournalTestSuite{})
}
