package server_monitoring_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/server_monitoring"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/common"
	_ "www.velocidex.com/golang/velociraptor/vql/filesystem"
	"www.velocidex.com/golang/velociraptor/vql/server/flows"
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
- query: SELECT * FROM register_run_count() WHERE log(message="Finished!", dedup=-1)
`, `
name: EventTest.Alert
type: SERVER_EVENT
sources:
- query: |
    SELECT * FROM scope()
    WHERE alert(name="TestAlert", field="Field1")
`, `
name: Server.Internal.Alerts
type: SERVER_EVENT
`}
)

type ServerMonitoringTestSuite struct {
	test_utils.TestSuite
	mu sync.Mutex
}

func (self *ServerMonitoringTestSuite) SetupTest() {
	self.mu.Lock()

	journal.PushRowsToArtifactAsyncIsSynchrnous = true

	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.MonitoringService = true

	self.LoadArtifactsIntoConfig(monitoringArtifacts)
	self.TestSuite.SetupTest()
}

func (self *ServerMonitoringTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	self.mu.Unlock()
}

func (self *ServerMonitoringTestSuite) TestMultipleArtifacts() {
	db := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)

	closer := utils.MockTime(utils.NewMockClock(time.Unix(1602103388, 0)))
	defer closer()

	event_table, err := services.GetServerEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Initially Server.Monitor.Health should be created if no
	// other config exists.
	configuration := &flows_proto.ArtifactCollectorArgs{}
	err = db.GetSubject(self.ConfigObj, paths.ServerMonitoringFlowURN, configuration)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(configuration.Artifacts))
	assert.Equal(self.T(), "Server.Monitor.Health", configuration.Artifacts[0])

	// Install the two event artifacts.
	err = event_table.Update(self.Ctx,
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
	event_table.(*server_monitoring.EventTable).Wait()

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

func (self *ServerMonitoringTestSuite) TestAlertEvent() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1602103388, 0)))
	defer closer()

	event_table, err := services.GetServerEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Install the two event artifacts.
	err = event_table.Update(self.Ctx,
		self.ConfigObj, "",
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"EventTest.Alert"},
		})
	assert.NoError(self.T(), err)

	// Wait here until all the queries are done.
	event_table.(*server_monitoring.EventTable).Wait()

	golden := ordereddict.NewDict()

	fs := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	for _, path := range []string{
		"/server_artifacts/Server.Internal.Alerts/2020-10-07.json",
		"/server_artifacts/EventTest.Alert/2020-10-07.json",
	} {
		value, pres := fs.Get(path)
		if pres {
			golden.Set(path, strings.Split(string(value), "\n"))
		}
	}
	goldie.AssertJson(self.T(), "TestAlertEvent", golden)
}

func (self *ServerMonitoringTestSuite) TestEmptyTable() {
	event_table, err := services.GetServerEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	closer := utils.MockTime(utils.NewMockClock(time.Unix(1602103388, 0)))
	defer closer()

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Add the new artifacts to the repository
	_, err = repository.LoadYaml(`
name: Sleep
sources:
- query: SELECT sleep(time=1000) FROM scope()
`, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)

	// Install a table with a sleep artifact.
	err = event_table.Update(self.Ctx,
		self.ConfigObj, "",
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Sleep"},
			Specs:     []*flows_proto.ArtifactSpec{},
		})
	assert.NoError(self.T(), err)

	// Wait until the query is installed.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return len(event_table.(*server_monitoring.EventTable).Tracer().Dump()) > 0
	})

	// Now install an empty table - all queries should quit.
	err = event_table.Update(self.Ctx,
		self.ConfigObj, "",
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{},
			Specs:     []*flows_proto.ArtifactSpec{},
		})
	assert.NoError(self.T(), err)

	// Wait until all queries are done.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		return len(event_table.(*server_monitoring.EventTable).Tracer().Dump()) == 0
	})
}

// Check that old event queries are properly shut down when table is
// updated.
func (self *ServerMonitoringTestSuite) TestQueriesAreCancelled() {
	run_count := int64(0)

	actions.QueryLog.Clear()

	// A new plugin to keep track of when a query is running - Total
	// number of runs is kept in run_count above.
	vql_subsystem.OverridePlugin(
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
	event_table, err := services.GetServerEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = event_table.Update(self.Ctx,
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
	err = event_table.Update(self.Ctx,
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
	tempdir, err := tempfile.TempDir("server_monitoring_test")
	assert.NoError(self.T(), err)

	defer os.RemoveAll(tempdir)

	closer := utils.MockTime(utils.NewMockClock(time.Unix(1602103388, 0)))
	defer closer()

	manager, err := services.GetRepositoryManager(self.ConfigObj)
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
`, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: false})

	assert.NoError(self.T(), err)

	event_table, err := services.GetServerEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Install a table with an initial artifact
	filename := filepath.Join(tempdir, "testfile1.txt")
	err = event_table.Update(self.Ctx,
		self.ConfigObj, utils.GetSuperuserName(self.ConfigObj),
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
	ctx := self.Ctx
	_, err = manager.SetArtifactFile(
		ctx, self.ConfigObj, "user", `
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

// Make sure watch monitoring can follow server event streams.
func (self *ServerMonitoringTestSuite) TestWatchMonitoring() {
	repository := self.LoadArtifacts(`
name: Test.Events
type: SERVER_EVENT
sources:
- query: SELECT * FROM clock()
`)

	event_table, err := services.GetServerEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Start collecting the events
	err = event_table.Update(self.Ctx,
		self.ConfigObj, utils.GetSuperuserName(self.ConfigObj),
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Test.Events"},
		})
	assert.NoError(self.T(), err)

	// Call the watch_monitoring plugin to ensure we can see them.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Repository: repository,
		Logger: logging.NewPlainLogger(
			self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)
	scope := manager.BuildScope(builder)
	defer scope.Close()

	subctx, cancel := context.WithTimeout(self.Ctx, 5*time.Second)
	defer cancel()

	var rows []vfilter.Row
	for row := range (&flows.WatchMonitoringPlugin{}).Call(
		subctx, scope, ordereddict.NewDict().
			Set("artifact", "Test.Events")) {

		// We only need one event
		rows = append(rows, row)
		break
	}
	assert.True(self.T(), len(rows) > 0)
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
