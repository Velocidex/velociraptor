package repository_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type ManagerTestSuite struct {
	test_utils.TestSuite
}

func (self *ManagerTestSuite) SetupTest() {
	self.TestSuite.SetupTest()
}

func (self *ManagerTestSuite) TestSetArtifact() {
	clock := &utils.MockClock{MockNow: time.Unix(1000000000, 0)}
	journal_manager, err := services.GetJournal()
	assert.NoError(self.T(), err)

	// Install a mock clock for this test.
	journal_manager.(*journal.JournalService).Clock = clock

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	// Coerce artifact into a prefix.
	artifact, err := manager.SetArtifactFile(self.ConfigObj, "User", `
name: TestArtifact
`, "Custom." /* required_prefix */)

	assert.NoError(self.T(), err)
	assert.Equal(self.T(), artifact.Name, "Custom.TestArtifact")

	file_store := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)

	data, pres := file_store.Get(
		"/artifact_definitions/Custom/TestArtifact.yaml")
	assert.True(self.T(), pres)

	assert.Contains(self.T(), string(data), "Custom.TestArtifact")

	// Make sure a creation event was written
	path_manager, err := artifacts.NewArtifactPathManager(
		self.ConfigObj, "", "", "Server.Internal.ArtifactModification")
	assert.NoError(self.T(), err)
	path_manager.Clock = clock

	data, pres = file_store.Get(
		path_manager.Path().AsFilestoreFilename(self.ConfigObj))
	assert.True(self.T(), pres)

	assert.Contains(self.T(), string(data), `"op":"set"`)
}

// On the minion the repository manager needs to be aware when new
// artifacts are created.
func (self *ManagerTestSuite) TestSetArtifactDetectedByMinion() {
	self.ConfigObj.Autoexec = &config_proto.AutoExecConfig{
		ArtifactDefinitions: []*artifacts_proto.Artifact{
			{
				Name: "Server.Internal.ArtifactModification",
				Type: "SERVER_EVENT",
			},
		},
	}

	clock := &utils.MockClock{MockNow: time.Unix(1000000000, 0)}
	journal_manager, err := services.GetJournal()
	assert.NoError(self.T(), err)

	// Install a mock clock for this test.
	journal_manager.(*journal.JournalService).Clock = clock

	// The global repository manager.
	err = repository.StartRepositoryManagerForTest(
		self.Sm.Ctx, self.Sm.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	master_manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	// Start another manager for the minion.

	// Spin up a minion client_info manager
	minion_config := proto.Clone(self.ConfigObj).(*config_proto.Config)
	minion_config.Frontend.IsMinion = true

	err = repository.StartRepositoryManagerForTest(
		self.Sm.Ctx, self.Sm.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	minion_manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	// Make sure they are not actually the same object.
	assert.NotEqual(self.T(),
		fmt.Sprintf("%p", minion_manager),
		fmt.Sprintf("%p", master_manager))

	// Coerce artifact into a prefix.
	artifact, err := master_manager.SetArtifactFile(self.ConfigObj, "User", `
name: TestArtifact
`, "")

	assert.NoError(self.T(), err)
	assert.Equal(self.T(), artifact.Name, "TestArtifact")

	minion_repository, err := minion_manager.GetGlobalRepository(minion_config)
	assert.NoError(self.T(), err)

	// Wait until the minion knows about the new artifact.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		_, ok := minion_repository.Get(minion_config, artifact.Name)
		return ok
	})

	// Now delete the artifact.
	err = master_manager.DeleteArtifactFile(self.ConfigObj, "User", "TestArtifact")
	assert.NoError(self.T(), err)

	// Wait until the minion removes it from its repository.
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		_, found := minion_repository.Get(minion_config, artifact.Name)
		return !found
	})
}

// If the artifact name already contains the prefix then prefix is not
// added.
func (self *ManagerTestSuite) TestSetArtifactWithExistingPrefix() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	// Coerce artifact into a prefix.
	artifact, err := manager.SetArtifactFile(self.ConfigObj, "User", `
name: Custom.TestArtifact
`, "Custom." /* required_prefix */)

	assert.NoError(self.T(), err)
	assert.Equal(self.T(), artifact.Name, "Custom.TestArtifact")

	file_store := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)

	data, pres := file_store.Get(
		"/artifact_definitions/Custom/TestArtifact.yaml")
	assert.True(self.T(), pres)

	assert.Contains(self.T(), string(data), "Custom.TestArtifact")
}

func (self *ManagerTestSuite) TestSetArtifactWithInvalidArtifact() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	// Invalid YAML
	_, err = manager.SetArtifactFile(self.ConfigObj, "User", `
nameXXXXX: Custom.TestArtifact
`, "Custom." /* required_prefix */)

	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "field nameXXXXX not found in type")

	// Valid YAML but invalid VQL
	_, err = manager.SetArtifactFile(self.ConfigObj, "User", `
name: Custom.TestArtifact
sources:
- query: "SELECT 1"
`, "Custom." /* required_prefix */)

	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "While parsing source query")
}

func (self *ManagerTestSuite) TestSetArtifactOverrideBuiltIn() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	// Try to override an existing artifact
	_, err = manager.SetArtifactFile(self.ConfigObj, "User", `
name: Generic.Client.Info
`, "" /* required_prefix */)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "Unable to override built in artifact")

	// Set Custom artifact
	_, err = manager.SetArtifactFile(self.ConfigObj, "User", `
name: Custom.Generic.Client.Info
`, "" /* required_prefix */)
	assert.NoError(self.T(), err)

	// Override it again
	_, err = manager.SetArtifactFile(self.ConfigObj, "User", `
name: Custom.Generic.Client.Info
`, "" /* required_prefix */)
	assert.NoError(self.T(), err)

	// Set Custom artifact with built_in in definition (this is a
	// private field which should be ignored).
	_, err = manager.SetArtifactFile(self.ConfigObj, "User", `
name: Custom.Generic.Client.Info
built_in: true
`, "" /* required_prefix */)
	assert.NoError(self.T(), err)

	// Override it again
	_, err = manager.SetArtifactFile(self.ConfigObj, "User", `
name: Custom.Generic.Client.Info
built_in: true
`, "" /* required_prefix */)
	assert.NoError(self.T(), err)
}

func TestManager(t *testing.T) {
	suite.Run(t, &ManagerTestSuite{})
}
