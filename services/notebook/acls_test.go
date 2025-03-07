package notebook_test

import (
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type ACLTestSuite struct {
	test_utils.TestSuite

	closer func()
}

func (self *ACLTestSuite) SetupTest() {
	mock_clock := utils.NewMockClock(time.Unix(10, 0))
	self.closer = utils.MockTime(mock_clock)

	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.NotebookService = true
	self.ConfigObj.Services.SchedulerService = true

	self.TestSuite.SetupTest()
}

func (self *ACLTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	self.closer()
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

	// User1 now has access
	assert.True(self.T(), notebook_manager.CheckNotebookAccess(new_notebook, "User1"))

	// What notebooks does User1 have access to?
	var all_rows []*ordereddict.Dict

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		index_filename, err := notebook_manager.GetSharedNotebooks(
			self.Ctx, "User1")
		assert.NoError(self.T(), err)

		all_rows = test_utils.FileReadRows(
			self.T(), self.ConfigObj, index_filename)

		return 1 == len(all_rows)
	})

	assert.Equal(self.T(), new_notebook.NotebookId,
		utils.GetString(all_rows[0], "NotebookId"))

	// Check GetAllNotebooks without ACL checks
	all_notebooks, err := notebook_manager.GetAllNotebooks(
		self.Ctx, services.NotebookSearchOptions{})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(all_rows))
	assert.Equal(self.T(), new_notebook.NotebookId,
		all_notebooks[0].NotebookId)

	// test_utils.GetMemoryDataStore(self.T(), self.ConfigObj).Debug()
}

func TestACLs(t *testing.T) {
	suite.Run(t, &ACLTestSuite{})
}
