package paths_test

import (
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// Run through examples of appropriate path placements.
func (self *PathManagerTestSuite) TestClientPathManager() {
	manager := paths.NewClientPathManager("C.123")
	assert.Equal(self.T(), "/ds/clients/C.123.db",
		self.getDatastorePath(manager.Path()))

	assert.Equal(self.T(), "/ds/clients/C.123/ping.json.db",
		self.getDatastorePath(manager.Ping()))

	assert.Equal(self.T(), "/ds/clients/C.123/labels.json.db",
		self.getDatastorePath(manager.Labels()))

	assert.Equal(self.T(), "/ds/clients/C.123/metadata.json.db",
		self.getDatastorePath(manager.Metadata()))

	assert.Equal(self.T(), "/ds/clients/C.123/key.db",
		self.getDatastorePath(manager.Key()))

	assert.Equal(self.T(), "/ds/clients/C.123/tasks/1234.db",
		self.getDatastorePath(manager.Task(1234)))

	// VFS data can contain arbitrary user data - the data store
	// escapes the paths heavily. It will also not be confused by
	// the .db extension. In this case we have the VFS path
	// contain components with path separators as well as Unicode
	// characters.
	var path_spec = manager.VFSPath([]string{
		"file", "\\\\.\\C:", "你好世界",
	})

	// AsClientPath produces a path string escaped with quotes for
	// path separators, rooted at the data store root.
	assert.Equal(self.T(), `/clients/C.123/vfs/file/"\\.\C:"/你好世界.json.db`,
		path_spec.AsClientPath())

	assert.Equal(self.T(), "/ds/clients/C.123/vfs/file/%5C%5C.%5CC%3A/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.json.db",
		self.getDatastorePath(path_spec))

	// For downloads we store a link file in the client's VFS pointing at the collection containing this information.
	path_spec = manager.VFSDownloadInfoPath([]string{
		// Path sep in the component name.
		"file", "\\\\.\\C:", "你好世界", "你好/世界.db",
	})

	data_store_path := "/ds/clients/C.123/vfs_files/file/%5C%5C.%5CC%3A/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C/%E4%BD%A0%E5%A5%BD%2F%E4%B8%96%E7%95%8C.db_.json.db"
	assert.Equal(self.T(), data_store_path,
		self.getDatastorePath(path_spec))
}
