package paths_test

import (
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *PathManagerTestSuite) TestUserPathManager() {
	manager := paths.NewUserPathManager("你好世界")

	assert.Equal(self.T(), "/ds/users/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.db",
		self.getDatastorePath(manager.Path()))

	assert.Equal(self.T(), "/ds/acl/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.json.db",
		self.getDatastorePath(manager.ACL()))

	assert.Equal(self.T(), "/ds/users/gui/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.json.db",
		self.getDatastorePath(manager.GUIOptions()))

	assert.Equal(self.T(), "/ds/users/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C/mru/C.1234.db",
		self.getDatastorePath(manager.MRUClient("C.1234")))

	assert.Equal(self.T(), "/ds/users/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C/Favorites/CLIENT/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.json.db",
		self.getDatastorePath(manager.Favorites("你好世界", "CLIENT")))
}
