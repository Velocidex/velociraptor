package server_artifacts_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/functions"
)

type ServerArtifactsTestSuite struct {
	test_utils.TestSuite
}

func (self *ServerArtifactsTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Frontend.ServerServices.ServerArtifacts = true

	self.TestSuite.SetupTest()

	// Create an administrator user
	err := services.GrantRoles(self.ConfigObj, "admin", []string{"administrator"})
	assert.NoError(self.T(), err)
}

func (self *ServerArtifactsTestSuite) LoadArtifacts(definition string) services.Repository {
	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)

	_, err := repository.LoadYaml(definition, false, true)
	assert.NoError(self.T(), err)

	return repository
}

func (self *ServerArtifactsTestSuite) ScheduleAndWait(
	name, user string) *api_proto.FlowDetails {

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)

	var mu sync.Mutex
	complete_flow_id := ""

	err := journal.WatchQueueWithCB(self.Sm.Ctx, self.ConfigObj, self.Sm.Wg,
		"System.Flow.Completion", "ServerArtifactsTestSuite", func(
			ctx context.Context,
			ConfigObj *config_proto.Config,
			row *ordereddict.Dict) error {
			flow, pres := row.Get("Flow")
			if pres {
				mu.Lock()
				complete_flow_id = flow.(*flows_proto.ArtifactCollectorContext).SessionId
				mu.Unlock()
			}
			return nil
		})
	assert.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	acl_manager := acl_managers.NewServerACLManager(self.ConfigObj, user)

	// Schedule a job for the server runner.
	flow_id, err := launcher.ScheduleArtifactCollection(
		self.Sm.Ctx, self.ConfigObj, acl_manager,
		repository, &flows_proto.ArtifactCollectorArgs{
			Creator:   user,
			ClientId:  "server",
			Artifacts: []string{name},
		}, func() {
			// Notify it about the new job
			notifier, err := services.GetNotifier(self.ConfigObj)
			assert.NoError(self.T(), err)

			err = notifier.NotifyListener(self.ConfigObj, "server", "")
			assert.NoError(self.T(), err)
		})
	assert.NoError(self.T(), err)

	// Wait for the collection to complete
	var details *api_proto.FlowDetails
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		details, err = launcher.GetFlowDetails(self.ConfigObj, "server", flow_id)
		assert.NoError(self.T(), err)

		return details.Context.State == flows_proto.ArtifactCollectorContext_FINISHED
	})

	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()
		return complete_flow_id == flow_id
	})

	return details
}

func (self *ServerArtifactsTestSuite) TestServerArtifacts() {
	self.LoadArtifacts(`
name: Test1
type: SERVER
sources:
- query: SELECT "Foo" FROM scope()
`)
	details := self.ScheduleAndWait("Test1", "admin")

	// One row is collected
	assert.Equal(self.T(), uint64(1), details.Context.TotalCollectedRows)

	// How long we took to run - should be immediate
	run_time := (details.Context.ActiveTime - details.Context.StartTime) / 1000000
	assert.True(self.T(), run_time < 2)
}

// Collect a long lived artifact with specified timeout.
func (self *ServerArtifactsTestSuite) TestServerArtifactsTimeout() {
	self.LoadArtifacts(`
name: Test2
type: SERVER
resources:
  timeout: 1
sources:
- query: SELECT sleep(time=200) FROM scope()
`)

	details := self.ScheduleAndWait("Test2", "admin")

	// No rows are collected because the query timed out.
	assert.Equal(self.T(), uint64(0), details.Context.TotalCollectedRows)

	// How long we took to run - should be around 1 second
	run_time := (details.Context.ActiveTime - details.Context.StartTime) / 1000000
	assert.True(self.T(), run_time < 3)
	assert.True(self.T(), run_time >= 1)

	flow_path_manager := paths.NewFlowPathManager(
		"server", details.Context.SessionId)
	log_data := test_utils.FileReadAll(self.T(), self.ConfigObj,
		flow_path_manager.Log())
	assert.Contains(self.T(), log_data, "Query timed out after ")
}

// The server artifact runner impersonates the flow creator for ACL
// checks - this makes it safe for low privilege users to run some
// server artifacts that accommodate their access levels, but stops
// them from escalating to higher permissions.
func (self *ServerArtifactsTestSuite) TestServerArtifactsACLs() {

	// The info plugin requires MACHINE_STATE permission
	self.LoadArtifacts(`
name: Test
type: SERVER
sources:
- query: SELECT * FROM info()
`)

	details := self.ScheduleAndWait("Test", "admin")

	// Admin user should be able to collect since it has EXECVE
	assert.Equal(self.T(), uint64(1), details.Context.TotalCollectedRows)

	// Create a reader user called gumby - reader role lacks the
	// MACHINE_STATE permission.
	err := services.GrantRoles(self.ConfigObj, "gumby", []string{"reader"})
	assert.NoError(self.T(), err)

	details = self.ScheduleAndWait("Test", "gumby")

	// Gumby user has no permissions to run the artifact.
	assert.Equal(self.T(), uint64(0), details.Context.TotalCollectedRows)

	flow_path_manager := paths.NewFlowPathManager(
		"server", details.Context.SessionId)
	log_data := test_utils.FileReadAll(self.T(), self.ConfigObj,
		flow_path_manager.Log())
	assert.Contains(self.T(), log_data, "Permission denied: [MACHINE_STATE]")
}

func TestServerArtifacts(t *testing.T) {
	suite.Run(t, &ServerArtifactsTestSuite{})
}
