package server_monitoring

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	_ "www.velocidex.com/golang/velociraptor/vql/common"
)

var (
	monitoringArtifact = `
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
`

	monitoringArtifact2 = `
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
`
)

type ServerMonitoringTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	sm         *services.Service
}

func (self *ServerMonitoringTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().
		WithWriteback().
		WithVerbose(true).
		LoadAndValidate()
	require.NoError(self.T(), err)

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	t := self.T()
	assert.NoError(t, self.sm.Start(journal.StartJournalService))
	assert.NoError(t, self.sm.Start(notifications.StartNotificationService))
	assert.NoError(t, self.sm.Start(launcher.StartLauncherService))
	assert.NoError(t, self.sm.Start(repository.StartRepositoryManager))
	assert.NoError(t, self.sm.Start(StartServerMonitoringService))
}

func (self *ServerMonitoringTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *ServerMonitoringTestSuite) TestMultipleArtifacts() {
	db := test_utils.GetMemoryDataStore(self.T(), self.config_obj)

	event_table := services.GetServerEventManager().(*EventTable)
	event_table.Clock = &utils.MockClock{MockNow: time.Unix(1602103388, 0)}

	// Initially Server.Monitor.Health should be created if no
	// other config exists.
	configuration, ok := db.Get("/config/server_monitoring.json").(*flows_proto.ArtifactCollectorArgs)
	assert.True(self.T(), ok)
	assert.Equal(self.T(), 1, len(configuration.Artifacts))
	assert.Equal(self.T(), "Server.Monitor.Health", configuration.Artifacts[0])

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.config_obj)
	assert.NoError(self.T(), err)

	// Add the new artifacts to the repository
	_, err = repository.LoadYaml(monitoringArtifact, true /* validate */)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(monitoringArtifact2, true /* validate */)
	assert.NoError(self.T(), err)

	// Install the two event artifacts.
	err = services.GetServerEventManager().Update(self.config_obj,
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
	configuration, ok = db.Get("/config/server_monitoring.json").(*flows_proto.ArtifactCollectorArgs)
	assert.True(self.T(), ok)
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

	fs := test_utils.GetMemoryFileStore(self.T(), self.config_obj)
	for _, path := range []string{
		"/server_artifacts/Server.Clock/2020-10-07.json",
		"/server_artifact_logs/Server.Clock/2020-10-07.json",

		"/server_artifacts/Server.Clock2/2020-10-07.json",
		"/server_artifact_logs/Server.Clock2/2020-10-07.json",
	} {
		value, pres := fs.Get(path)
		if pres {
			golden.Set(path, strings.Split(string(value), "\n"))
		}
	}

	for _, path := range []string{
		"/config/server_monitoring.json",
	} {
		golden.Set(path, db.Get(path))
	}

	golden_str := json.MustMarshalIndent(golden)
	golden_str = regexp.MustCompile("Query Stats.+").ReplaceAll(golden_str, []byte{})

	goldie.Assert(self.T(), "TestMultipleArtifacts", golden_str)
}

func TestServerMonitoring(t *testing.T) {
	suite.Run(t, &ServerMonitoringTestSuite{})
}
