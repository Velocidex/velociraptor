package paths_test

import (
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *PathManagerTestSuite) TestArtifactPathManager() {
	assert.Equal(self.T(),
		"/fs/artifact_definitions/Windows/Some/Artifact.yaml",
		self.getFilestorePath(
			paths.GetArtifactDefintionPath("Windows.Some.Artifact")))

}
