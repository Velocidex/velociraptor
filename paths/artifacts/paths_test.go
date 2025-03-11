package artifacts_test

import (
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"

	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
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
	test_utils.TestSuite
	dirname string
}

func (self *PathManageTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()

	var err error
	self.dirname, err = tempfile.TempDir("path_manager_test")
	assert.NoError(self.T(), err)

	self.ConfigObj.Datastore.Implementation = "FileBaseDataStore"
	self.ConfigObj.Datastore.FilestoreDirectory = self.dirname
	self.ConfigObj.Datastore.Location = self.dirname

	self.LoadArtifactsIntoConfig([]string{`
name: Windows.Sys.Users
type: CLIENT
`, `
name: Server.Utils.CreateCollector
type: SERVER
`, `
name: Elastic.Flows.Upload
type: SERVER_EVENT
`, `
name: Windows.Events.ProcessCreation
type: CLIENT_EVENT
`})

	self.TestSuite.SetupTest()

}

func (self *PathManageTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()

	os.RemoveAll(self.dirname) // clean up
}

// The path manager maps artifacts, clients, flows etc into a file
// store path. For event artifacts, the path manager splits the files
// by day to ensure they are not too large and can be easily archived.
func (self *PathManageTestSuite) TestPathManager() {
	ts := int64(1587800823)
	closer := utils.MockTime(utils.NewMockClock(time.Unix(ts, 0)))
	defer closer()

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	for _, testcase := range path_tests {
		path_manager, err := artifacts.NewArtifactPathManager(
			self.Ctx, self.ConfigObj,
			testcase.client_id,
			testcase.flow_id,
			testcase.full_artifact_name)
		assert.NoError(self.T(), err)

		path, err := path_manager.GetPathForWriting()
		assert.NoError(self.T(), err)
		assert.Equal(self.T(),
			cleanPath(datastore.AsFilestoreFilename(
				db, self.ConfigObj, path)),
			cleanPath(self.dirname+"/"+testcase.expected))

		file_store_factory := memory.NewMemoryFileStore(self.ConfigObj)
		qm := memory.NewMemoryQueueManager(
			self.ConfigObj, file_store_factory).(*memory.MemoryQueueManager)

		file_store.OverrideFilestoreImplementation(self.ConfigObj, file_store_factory)

		err = qm.PushEventRows(path_manager,
			[]*ordereddict.Dict{ordereddict.NewDict()})
		assert.NoError(self.T(), err)

		data, ok := file_store_factory.Get(cleanPath(
			self.dirname + testcase.expected))
		assert.Equal(self.T(), ok, true)
		assert.Equal(self.T(), string(data), "{\"_ts\":1587800823}\n")
	}
}

func TestPathTest(t *testing.T) {
	suite.Run(t, &PathManageTestSuite{})
}

func cleanPath(in string) string {
	if runtime.GOOS == "windows" {
		return path.Clean(strings.Replace(strings.TrimPrefix(
			in, path_specs.WINDOWS_LFN_PREFIX), "\\", "/", -1))
	}
	return path.Clean(in)
}
