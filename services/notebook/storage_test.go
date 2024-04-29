package notebook_test

import (
	"github.com/alecthomas/assert"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *NotebookManagerTestSuite) TestNotebookStorage() {
	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Create a notebook the usual way.
	global_notebook, err := notebook_manager.NewNotebook(
		self.Ctx, "admin", &api_proto.NotebookMetadata{
			Name: "Test Global Notebook",
		})
	assert.NoError(self.T(), err)

	// Now create a flow notebook - these have pre-determined ID
	_, err = notebook_manager.NewNotebook(
		self.Ctx, "admin", &api_proto.NotebookMetadata{
			Name:       "Test Flow Notebook",
			NotebookId: "N.F.CODU1SDAMQ3CM-C.3ece159995d35b34",
		})
	assert.NoError(self.T(), err)

	// Now list all notebooks - should only see the global one.
	notebooks, err := notebook_manager.GetAllNotebooks()
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 1, len(notebooks))
	assert.Equal(self.T(), global_notebook.NotebookId, notebooks[0].NotebookId)
}
