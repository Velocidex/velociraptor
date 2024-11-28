package notebook_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/services/scheduler"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var (
	mock_definitions = []string{`
name: Server.Internal.ArtifactDescription
type: SERVER
`, `
name: Server.Internal.Alerts
type: SERVER_EVENT
`}
)

type NotebookManagerTestSuite struct {
	test_utils.TestSuite
	closer func()
}

func (self *NotebookManagerTestSuite) SetupTest() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.NotebookService = true
	self.ConfigObj.Services.SchedulerService = true

	// Keep 3 versions of each cell
	self.ConfigObj.Defaults.NotebookVersions = 3

	self.LoadArtifactsIntoConfig(mock_definitions)

	// Stop automatic syncing
	notebook.DO_NOT_SYNC_NOTEBOOKS_FOR_TEST.Store(true)

	self.TestSuite.SetupTest()

	// Mock out cell ID generation for tests
	self.closer = func() {
		closer()
	}

	// Create an administrator user
	err := services.GrantRoles(self.ConfigObj, "admin", []string{"administrator"})
	assert.NoError(self.T(), err)
}

func (self *NotebookManagerTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	self.closer()
}

func (self *NotebookManagerTestSuite) TestNotebookManagerUpdateCell() {
	assert.Retry(self.T(), 3, time.Second, self._TestNotebookManagerUpdateCell)
}

func (self *NotebookManagerTestSuite) _TestNotebookManagerUpdateCell(r *assert.R) {
	// Mock out cell ID generation for tests
	gen := utils.IncrementalIdGenerator(0)
	closer := utils.SetIdGenerator(&gen)
	defer closer()

	scheduler_service, err := services.GetSchedulerService(self.ConfigObj)
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(2*time.Second, r, func() bool {
		return scheduler_service.(*scheduler.Scheduler).AvailableWorkers() > 0
	})

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(r, err)

	golden := ordereddict.NewDict()

	// Create a notebook the usual way.
	var notebook *api_proto.NotebookMetadata
	vtesting.WaitUntil(2*time.Second, r, func() bool {
		notebook, err = notebook_manager.NewNotebook(
			self.Ctx, "admin", &api_proto.NotebookMetadata{
				Name:        "Test Notebook",
				Description: "This is a test",
			})
		return err == nil
	})

	assert.Equal(r, len(notebook.CellMetadata), 1)
	assert.Equal(r, notebook.CellMetadata[0].CurrentVersion, "03")
	assert.Equal(r, notebook.CellMetadata[0].AvailableVersions, []string{"03"})

	golden.Set("Notebook Metadata", notebook)

	// Now update the cell to some markdown
	cell, err := notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
		"admin", &api_proto.NotebookCellRequest{
			NotebookId: notebook.NotebookId,
			CellId:     notebook.CellMetadata[0].CellId,
			Input:      "# Heading 1\n\nHello world\n",
			Type:       "MarkDown",
		})
	assert.NoError(r, err)

	golden.Set("Markdown Cell", cell)

	cell, err = notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
		"admin", &api_proto.NotebookCellRequest{
			NotebookId: notebook.NotebookId,
			CellId:     notebook.CellMetadata[0].CellId,
			Input:      "SELECT _value AS X FROM range(end=2)",
			Type:       "VQL",
		})
	assert.NoError(r, err)

	// The new cell should have a higher version
	assert.Equal(r, len(notebook.CellMetadata), 1)
	assert.Equal(r, cell.CurrentVersion, "05")

	// The old version is still there and available. There should be 3
	// versions all up.
	assert.Equal(r, cell.AvailableVersions, []string{"03", "04", "05"})

	// The cell that is returned from the UpdateNotebookCell contains
	// all the data.
	golden.Set("VQL Cell", cell)

	// The notebook itself should only contain summary cells.
	new_notebook, err := notebook_manager.GetNotebook(
		self.Ctx, notebook.NotebookId, services.DO_NOT_INCLUDE_UPLOADS)
	assert.NoError(r, err)
	golden.Set("Full Notebook after update", new_notebook)

	goldie.Retry(r, self.T(), "TestNotebookManagerUpdateCell",
		goldie.RemoveLines("timestamp", json.MustMarshalIndent(golden)))
}

func (self *NotebookManagerTestSuite) TestNotebookManagerAlert() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(100, 10)))
	defer closer()

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Create a notebook the usual way.
	var notebook *api_proto.NotebookMetadata
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		notebook, err = notebook_manager.NewNotebook(self.Ctx, "admin", &api_proto.NotebookMetadata{
			Name:        "Test Notebook",
			Description: "This is a test",
		})
		return err == nil
	})

	// Now update the cell to some markdown
	_, err = notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
		"admin", &api_proto.NotebookCellRequest{
			NotebookId: notebook.NotebookId,
			CellId:     notebook.CellMetadata[0].CellId,
			Input:      "SELECT alert(name='My Alert', Context='Something went wrong!') FROM scope()",
			Type:       "vql",
		})
	assert.NoError(self.T(), err)

	mem_file_store := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)

	// Make sure the alert is sent.
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		alert, _ := mem_file_store.Get("/server_artifacts/Server.Internal.Alerts/1970-01-01.json")
		return strings.Contains(string(alert), `"name":"My Alert"`)
	})
}

func TestNotebookManager(t *testing.T) {
	suite.Run(t, &NotebookManagerTestSuite{})
}
