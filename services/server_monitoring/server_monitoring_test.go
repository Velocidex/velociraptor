package server_monitoring

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	_ "www.velocidex.com/golang/velociraptor/vql/common"
	_ "www.velocidex.com/golang/velociraptor/vql/filesystem"
)

var (
	monitoringArtifacts = []string{`
name: Server.Clock
type: SERVER_EVENT
parameters:
- name: Foo
  default: Should Be Overriden
- name: BoolFoo
  type: bool
- name: Foo2
  default: DefaultFoo2
sources:
- query: |
     SELECT Foo, Foo2, BoolFoo FROM clock(ms=10)
     LIMIT 5
`, `
name: Server.Clock2
type: SERVER_EVENT
parameters:
- name: Foo2
  default: AnotherFoo2
- name: Foo
  default: FooValue
sources:
- query: |
     SELECT Foo, Foo2 FROM clock(ms=10)
     LIMIT 5
`, `
name: WaitForCancel
type: SERVER_EVENT
sources:
- query: SELECT * FROM register_run_count() WHERE log(message="Finished!")
`}
)

type ServerMonitoringTestSuite struct {
	test_utils.TestSuite
}

func (self *ServerMonitoringTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	self.LoadArtifacts(monitoringArtifacts)

	assert.NoError(self.T(), self.Sm.Start(StartServerMonitoringService))
}

func (self *ServerMonitoringTestSuite) TestMultipleArtifacts() {
	db := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)

	event_table := services.GetServerEventManager().(*EventTable)
	event_table.SetClock(&utils.MockClock{MockNow: time.Unix(1602103388, 0)})

	// Initially Server.Monitor.Health should be created if no
	// other config exists.
	configuration := &flows_proto.ArtifactCollectorArgs{}
	err := db.GetSubject(self.ConfigObj, paths.ServerMonitoringFlowURN, configuration)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(configuration.Artifacts))
	assert.Equal(self.T(), "Server.Monitor.Health", configuration.Artifacts[0])

	// Install the two event artifacts.
	err = services.GetServerEventManager().Update(
		self.ConfigObj, "",
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Server.Clock", "Server.Clock2"},
			Specs: []*flows_proto.ArtifactSpec{
				{
					Artifact: "Server.Clock",
					Parameters: &flows_proto.ArtifactParameters{
						Env: []*actions_proto.VQLEnv{
							{Key: "Foo", Value: "Y"},
							{Key: "BoolFoo", Value: "Y"},
						},
					},
				},
			},
		})
	assert.NoError(self.T(), err)

	// Make sure the new configuration is written to disk
	err = db.GetSubject(self.ConfigObj, paths.ServerMonitoringFlowURN, configuration)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 2, len(configuration.Artifacts))
	assert.Equal(self.T(), "Server.Clock", configuration.Artifacts[0])

	// Wait here until all the queries are done.
	event_table.wg.Wait()

	// Expected Server.Clock rows:
	// {"Foo":"Y","Foo2":"DefaultFoo2","BoolFoo":true,"_ts":1602103388}

	// Foo is overridden, Foo2 is default, BoolFoo is converted from "Y"

	// Expected Server.Clock2 rows:
	// {"Foo":"FooValue","Foo2":"AnotherFoo2","_ts":1602103388}

	// All values are default.

	golden := ordereddict.NewDict()

	fs := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	for _, path := range []string{
		"/server_artifacts/Server.Clock/2020-10-07.json",
		"/server_artifact_logs/Server.Clock/2020-10-07.json",

		// Make sure files have time indexes.
		"/server_artifacts/Server.Clock/2020-10-07.json.tidx",
		"/server_artifact_logs/Server.Clock/2020-10-07.json.tidx",

		"/server_artifacts/Server.Clock2/2020-10-07.json",
		"/server_artifact_logs/Server.Clock2/2020-10-07.json",
	} {
		value, pres := fs.Get(path)
		if pres {
			if strings.HasSuffix(path, ".tidx") {
				golden.Set(path, fmt.Sprintf("% x", value))
			} else {
				golden.Set(path, strings.Split(string(value), "\n"))
			}
		}
	}

	golden.Set(paths.ServerMonitoringFlowURN.AsClientPath(), configuration)

	golden_str := json.MustMarshalIndent(golden)
	golden_str = regexp.MustCompile("Query Stats.+").ReplaceAll(golden_str, []byte{})

	goldie.Assert(self.T(), "TestMultipleArtifacts", golden_str)
}

func (self *ServerMonitoringTestSuite) TestEmptyTable() {
	event_table := services.GetServerEventManager().(*EventTable)
	event_table.SetClock(&utils.MockClock{MockNow: time.Unix(1602103388, 0)})

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Add the new artifacts to the repository
	_, err = repository.LoadYaml(`
name: Sleep
sources:
- query: SELECT sleep(time=1000) FROM scope()
`, true /* validate */, true)
	assert.NoError(self.T(), err)

	// Install a table with a sleep artifact.
	err = services.GetServerEventManager().Update(
		self.ConfigObj, "",
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Sleep"},
			Specs:     []*flows_proto.ArtifactSpec{},
		})
	assert.NoError(self.T(), err)

	// Wait until the query is installed.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return len(event_table.Tracer().Dump()) > 0
	})

	// Now install an empty table - all queries should quit.
	err = services.GetServerEventManager().Update(
		self.ConfigObj, "",
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{},
			Specs:     []*flows_proto.ArtifactSpec{},
		})
	assert.NoError(self.T(), err)

	// Wait until all queries are done.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return len(event_table.Tracer().Dump()) == 0
	})
}

// Check that old event queries are properly shut down when table is
// updated.
func (self *ServerMonitoringTestSuite) TestQueriesAreCancelled() {
	run_count := int64(0)

	actions.QueryLog.Clear()

	// A new plugin to keep track of when a query is running - Total
	// number of runs is kept in run_count above.
	vql_subsystem.RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "register_run_count",
			Function: func(
				ctx context.Context, scope vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {

				atomic.AddInt64(&run_count, 1)

				// Wait here until we get cancelled.
				<-ctx.Done()
				atomic.AddInt64(&run_count, -1)

				return nil
			},
		})

	// Install a table with a an artifact that uses the plugin.
	err := services.GetServerEventManager().Update(
		self.ConfigObj, "",
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"WaitForCancel"},
			Specs:     []*flows_proto.ArtifactSpec{},
		})
	assert.NoError(self.T(), err)

	// Wait here until the query is installed.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return atomic.LoadInt64(&run_count) == 1
	})

	// Now install an empty table - all queries should quit.
	err = services.GetServerEventManager().Update(
		self.ConfigObj, "",
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{},
			Specs:     []*flows_proto.ArtifactSpec{},
		})
	assert.NoError(self.T(), err)

	// Wait until all queries are done and cancelled.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return atomic.LoadInt64(&run_count) == 0
	})
}

func (self *ServerMonitoringTestSuite) TestUpdateWhenArtifactModified() {
	tempdir, err := ioutil.TempDir("", "server_monitoring_test")
	assert.NoError(self.T(), err)

	defer os.RemoveAll(tempdir)

	event_table := services.GetServerEventManager().(*EventTable)
	event_table.SetClock(&utils.MockClock{MockNow: time.Unix(1602103388, 0)})

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Add the new custom artifacts to the repository
	_, err = repository.LoadYaml(`
name: TestArtifact
type: SERVER_EVENT
parameters:
- name: Filename
sources:
- query: |
   SELECT copy(accessor='data', filename='hello', dest=Filename)
   FROM scope()
`, true /* validate */, false /* built in */)
	assert.NoError(self.T(), err)

	// Install a table with an initial artifact
	filename := filepath.Join(tempdir, "testfile1.txt")
	err = services.GetServerEventManager().Update(
		self.ConfigObj, "VelociraptorServer",
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact"},
			Specs: []*flows_proto.ArtifactSpec{{
				Artifact: "TestArtifact",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{{
						Key:   "Filename",
						Value: filename,
					}},
				}},
			},
		})
	assert.NoError(self.T(), err)

	// Wait until the query is actually run - the file should be
	// created with the text "hello" in it.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return readAll(filename) == "hello"
	})

	// Now we update the artifact definition and the monitoring table
	// should magically be updated and the new artifact run instead.
	_, err = manager.SetArtifactFile(self.ConfigObj, "user", `
name: TestArtifact
type: SERVER_EVENT
parameters:
- name: Filename
sources:
- query: |
   SELECT copy(accessor='data', filename='goodbye', dest=Filename)
   FROM scope()
`, "")
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return readAll(filename) == "goodbye"
	})
}

func TestServerMonitoring(t *testing.T) {
	suite.Run(t, &ServerMonitoringTestSuite{})
}

func readAll(filename string) string {
	fd, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return ""
	}

	return string(data)
}
