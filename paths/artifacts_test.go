package paths_test

import (
	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/velociraptor/paths"
)

func (self *PathManagerTestSuite) TestArtifactPathManager() {
	assert.Equal(self.T(),
		"/fs/artifact_definitions/Windows/Some/Artifact.yaml",
		self.getFilestorePath(
			paths.GetArtifactDefintionPath("Windows.Some.Artifact")))

}
