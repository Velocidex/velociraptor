package paths_test

import (
	"time"

	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *PathManagerTestSuite) TestNotebookPathManager() {
	manager := paths.NewNotebookPathManager("N.123")
	manager.Clock = &utils.MockClock{
		MockNow: time.Unix(1000000000, 0).UTC(),
	}

	assert.Equal(self.T(), "/ds/notebooks/N.123.json.db",
		self.getDatastorePath(manager.Path()))

	assert.Equal(self.T(), "/fs/notebooks/N.123/files/NA.123%2Fimage.png",
		self.getFilestorePath(manager.Attachment("NA.123/image.png")))

	// Exports are available in the (authenticated) downloads directory.
	assert.Equal(self.T(), "/fs/downloads/notebooks/N.123/N.123-20010909014640Z.html",
		self.getFilestorePath(manager.HtmlExport()))

	assert.Equal(self.T(), "/fs/downloads/notebooks/N.123/N.123-20010909014640Z.zip",
		self.getFilestorePath(manager.ZipExport()))

	// Get a cell in the notebook
	cell_manager := manager.Cell("C.123")
	assert.Equal(self.T(), "/ds/notebooks/N.123/C.123.json.db",
		self.getDatastorePath(cell_manager.Path()))

	// Store the query results in the cell
	query_manager := cell_manager.QueryStorage(1)
	assert.Equal(self.T(), "/fs/notebooks/N.123/C.123/query_1.json",
		self.getFilestorePath(query_manager.Path()))

}
