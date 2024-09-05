package paths_test

import (
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *PathManagerTestSuite) TestHuntPathManager() {
	manager := paths.NewHuntPathManager("H.1234")
	assert.Equal(self.T(), "/ds/hunts/H.1234.db",
		self.getDatastorePath(manager.Path()))

	assert.Equal(self.T(), "/fs/downloads/hunts/H.1234/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8CH.1234-summary.zip",
		self.getFilestorePath(manager.GetHuntDownloadsFile(
			true /* only_combined */, "你好世界", false,
		)))

	assert.Equal(self.T(), "/fs/hunts/H.1234.json",
		self.getFilestorePath(manager.Clients()))

	assert.Equal(self.T(), "/fs/hunts/H.1234_errors.json",
		self.getFilestorePath(manager.ClientErrors()))
}
