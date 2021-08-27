package repository_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type ManagerTestSuite struct {
	test_utils.TestSuite
}

func (self *ManagerTestSuite) SetupTest() {
	self.TestSuite.SetupTest()
}

func (self *ManagerTestSuite) TestSetArtifact() {
	clock := utils.MockClock{MockNow: time.Unix(1000000000, 0)}
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

func TestManager(t *testing.T) {
	suite.Run(t, &ManagerTestSuite{})
}
