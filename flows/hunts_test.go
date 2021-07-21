package flows

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type HuntTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	sm         *services.Service
	ctx        context.Context
}

func (self *HuntTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	// Start essential services.
	self.ctx, _ = context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(self.ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.sm.Start(hunt_dispatcher.StartHuntDispatcher))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
}

func (self *HuntTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *HuntTestSuite) TestCompilation() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository := manager.NewRepository()
	manager.SetGlobalRepositoryForTests(self.config_obj, repository)
	repository.LoadYaml(`
name: TestArtifact
parameters:
- name: TestArtifact_Arg1
  default: TestArtifact_Arg1_Value

sources:
- query:
    SELECT * FROM info()
`, true)

	repository.LoadYaml(`
name: System.Hunt.Creation
type: SERVER_EVENT`, true)

	repository.LoadYaml(`
name: AnotherTestArtifact
parameters:
- name: AnotherTestArtifact_Arg1
  default: AnotherTestArtifact_Arg1_Value

sources:
- query:
    SELECT * FROM scope()
`, true)
	request := &api_proto.Hunt{
		HuntDescription: "My hunt",
		StartRequest: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact", "AnotherTestArtifact"},
		},
	}

	acl_manager := vql_subsystem.NullACLManager{}
	hunt_id, err := CreateHunt(self.ctx, self.config_obj, acl_manager, request)

	assert.NoError(self.T(), err)

	db := test_utils.GetMemoryDataStore(self.T(), self.config_obj)
	hunt_obj, pres := db.Subjects["/hunts/"+hunt_id].(*api_proto.Hunt)
	assert.True(self.T(), pres)

	assert.Equal(self.T(), hunt_obj.HuntDescription, request.HuntDescription)
	assert.NotEqual(self.T(), hunt_obj.CreateTime, uint64(0))

	// Hunts are created when paused
	assert.Equal(self.T(), hunt_obj.State, api_proto.Hunt_PAUSED)

	// Check that the hunt caches the compiled collector args so
	// we dont need to compile it for each client. For normal
	// artifacts there should be only one collector args because
	// each artifact is collected serially on the client.
	assert.Equal(self.T(), len(hunt_obj.StartRequest.CompiledCollectorArgs), 2)

	keys := []string{}
	for _, compiled := range hunt_obj.StartRequest.CompiledCollectorArgs {
		for _, env := range compiled.Env {
			keys = append(keys, env.Key)
		}
	}

	assert.Equal(self.T(), keys,
		[]string{"TestArtifact_Arg1", "AnotherTestArtifact_Arg1"})
}

func TestHunts(t *testing.T) {
	suite.Run(t, &HuntTestSuite{})
}
