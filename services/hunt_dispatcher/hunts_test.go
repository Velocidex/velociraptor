package hunt_dispatcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type HuntTestSuite struct {
	test_utils.TestSuite
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
`, true, true)

	repository.LoadYaml(`
name: System.Hunt.Creation
type: SERVER_EVENT`, true, true)

	repository.LoadYaml(`
name: AnotherTestArtifact
parameters:
- name: AnotherTestArtifact_Arg1
  default: AnotherTestArtifact_Arg1_Value

sources:
- query:
    SELECT * FROM scope()
`, true, true)
	request := &api_proto.Hunt{
		HuntDescription: "My hunt",
		StartRequest: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact", "AnotherTestArtifact"},
		},
	}

	acl_manager := vql_subsystem.NullACLManager{}
	hunt_dispatcher := services.GetHuntDispatcher()
	hunt_id, err := hunt_dispatcher.CreateHunt(
		self.ctx, self.config_obj, acl_manager, request)

	assert.NoError(self.T(), err)

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	hunt_obj := &api_proto.Hunt{}
	err = db.GetSubject(self.config_obj, hunt_path_manager.Path(), hunt_obj)
	assert.NoError(self.T(), err)

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
