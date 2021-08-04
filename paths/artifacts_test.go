package paths_test

import (
	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/velociraptor/paths"
)

func (self *PathManagerTestSuite) TestArtifactPathManager() {
	assert.Equal(self.T(),
		"/ds/artifact_definitions/Windows/Some/Artifact.yaml",
		self.getDatastorePath(
			paths.GetArtifactDefintionPath("Windows.Some.Artifact")))

}
