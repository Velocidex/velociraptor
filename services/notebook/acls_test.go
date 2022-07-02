package notebook_test

import (
	"testing"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/notebook"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type ACLTestSuite struct {
	test_utils.TestSuite
}

func (self *ACLTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Frontend.ServerServices.NotebookService = true

	self.TestSuite.SetupTest()
}

func (self *ACLTestSuite) TestNotebookPublicACL() {
	new_notebook := &api_proto.NotebookMetadata{
		NotebookId: "N.12345",
		Creator:    "Creator",
		Public:     true,
	}

	notebook_manager_any, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	notebook_manager := notebook_manager_any.(*notebook.NotebookManager)

	err = notebook_manager.Store.SetNotebook(new_notebook)
	assert.NoError(self.T(), err)

	// Check that everyone has access
	assert.True(self.T(), notebook_manager.CheckNotebookAccess(new_notebook, "User1"))

	// Make the notebook not public.
	new_notebook.Public = false

	err = notebook_manager.Store.SetNotebook(new_notebook)
	assert.NoError(self.T(), err)

	// User1 lost access.
	assert.False(self.T(), notebook_manager.CheckNotebookAccess(
		new_notebook, "User1"))

	// The creator always has access regardless
	assert.True(self.T(), notebook_manager.CheckNotebookAccess(new_notebook, "Creator"))

	// Explicitly share with User1
	new_notebook.Collaborators = append(new_notebook.Collaborators, "User1")
	err = notebook_manager.Store.SetNotebook(new_notebook)
	assert.NoError(self.T(), err)

	err = notebook_manager.Store.UpdateShareIndex(new_notebook)
	assert.NoError(self.T(), err)

	// User1 now has access
	assert.True(self.T(), notebook_manager.CheckNotebookAccess(new_notebook, "User1"))

	// What notebooks does User1 have access to?
	notebooks, err := notebook_manager.GetSharedNotebooks(self.Sm.Ctx, "User1", 0, 100)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(notebooks))
	assert.Equal(self.T(), new_notebook.NotebookId, notebooks[0].NotebookId)

	// Check GetAllNotebooks without ACL checks
	all_notebooks, err := notebook.GetAllNotebooks(self.ConfigObj)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(notebooks))
	assert.Equal(self.T(), new_notebook.NotebookId, all_notebooks[0].NotebookId)

	// test_utils.GetMemoryDataStore(self.T(), self.ConfigObj).Debug()
}

func TestACLs(t *testing.T) {
	suite.Run(t, &ACLTestSuite{})
}
