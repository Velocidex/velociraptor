package paths_test

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *PathManagerTestSuite) TestTimelinePathManager() {
	// Put the timeline in a notebook
	manager := NewSuperTimelinePathManager(
		"你好世界/hello",
		paths.NewNotebookPathManager("N.123").SuperTimelineDir())

	assert.Equal(self.T(), "/ds/notebooks/N.123/timelines/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C%2Fhello.json.db",
		self.getDatastorePath(manager.Path()))

	// Create a child timeline
	child_manager := manager.GetChild("你好世界")

	assert.Equal(self.T(), "/fs/notebooks/N.123/timelines/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C%2Fhello/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.json",
		self.getFilestorePath(child_manager.Path()))

	assert.Equal(self.T(), "/fs/notebooks/N.123/timelines/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C%2Fhello/%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.json.tidx",
		self.getFilestorePath(child_manager.Index()))
}

func NewSuperTimelinePathManager(
	name string, root api.DSPathSpec) *paths.SuperTimelinePathManager {
	return &paths.SuperTimelinePathManager{
		Name: name,
		Root: root,
	}
}
