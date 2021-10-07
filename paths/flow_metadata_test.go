package paths_test

import (
	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/velociraptor/paths"
)

func (self *PathManagerTestSuite) TestFlowPathManager() {
	manager := paths.NewFlowPathManager("C.123", "F.1234")

	assert.Equal(self.T(), "/ds/clients/C.123/collections/F.1234.db",
		self.getDatastorePath(manager.Path()))

	assert.Equal(self.T(), "/fs/clients/C.123/collections/F.1234/logs",
		self.getFilestorePath(manager.Log()))

	assert.Equal(self.T(), "/ds/clients/C.123/collections/F.1234/task.db",
		self.getDatastorePath(manager.Task()))

	assert.Equal(self.T(), "/fs/clients/C.123/collections/F.1234/uploads.json",
		self.getFilestorePath(manager.UploadMetadata()))

	assert.Equal(self.T(), "/fs/downloads/C.123/F.1234/HostnameX-C.123-F.1234.zip",
		self.getFilestorePath(manager.GetDownloadsFile("HostnameX", false)))

	assert.Equal(self.T(), "/fs/downloads/C.123/F.1234/Report HostnameX-C.123-F.1234.html",
		self.getFilestorePath(manager.GetReportsFile("HostnameX")))

	assert.Equal(self.T(), "/fs/clients/C.123/collections/F.1234/uploads/ntfs/c%3A/Windows/System32/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.txt",
		self.getFilestorePath(manager.GetUploadsFile(
			"ntfs", `\\c:\Windows\System32\你好世界.txt`).Path()))

}
