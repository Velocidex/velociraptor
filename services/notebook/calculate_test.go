package notebook_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
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
	self.closer = utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.NotebookService = true
	self.ConfigObj.Services.SchedulerService = true

	// Keep 3 versions of each cell
	self.ConfigObj.Defaults.NotebookVersions = 3

	self.LoadArtifactsIntoConfig(mock_definitions)

	self.TestSuite.SetupTest()

	// Mock out cell ID generation for tests
	gen := utils.IncrementalIdGenerator(0)
	utils.SetIdGenerator(&gen)
}

func (self *NotebookManagerTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	self.closer()
}

func (self *NotebookManagerTestSuite) TestNotebookManagerUpdateCell() {
	// Mock out cell ID generation for tests
	gen := utils.IncrementalIdGenerator(0)
	utils.SetIdGenerator(&gen)

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict()

	// Create a notebook the usual way.
	var notebook *api_proto.NotebookMetadata
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		notebook, err = notebook_manager.NewNotebook(self.Ctx, "admin", &api_proto.NotebookMetadata{
			Name:        "Test Notebook",
			Description: "This is a test",
		})
		return err == nil
	})

	assert.Equal(self.T(), len(notebook.CellMetadata), 1)
	assert.Equal(self.T(),
		notebook.CellMetadata[0].CurrentVersion, "04")
	assert.Equal(self.T(),
		notebook.CellMetadata[0].AvailableVersions, []string{"04"})

	golden.Set("Notebook Metadata", notebook)

	// Now update the cell to some markdown
	cell, err := notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
		"admin", &api_proto.NotebookCellRequest{
			NotebookId: notebook.NotebookId,
			CellId:     notebook.CellMetadata[0].CellId,
			Input:      "# Heading 1\n\nHello world\n",
			Type:       "MarkDown",
		})
	assert.NoError(self.T(), err)

	golden.Set("Markdown Cell", cell)

	cell, err = notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
		"admin", &api_proto.NotebookCellRequest{
			NotebookId: notebook.NotebookId,
			CellId:     notebook.CellMetadata[0].CellId,
			Input:      "SELECT _value AS X FROM range(end=2)",
			Type:       "VQL",
		})
	assert.NoError(self.T(), err)

	// The new cell should have a higher version
	assert.Equal(self.T(), len(notebook.CellMetadata), 1)
	assert.Equal(self.T(), cell.CurrentVersion, "06")

	// The old version is still there and available. There should be 3
	// versions all up.
	assert.Equal(self.T(), cell.AvailableVersions, []string{"04", "05", "06"})

	golden.Set("VQL Cell", cell)
	goldie.Assert(self.T(), "TestNotebookManagerUpdateCell",
		json.MustMarshalIndent(golden))
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
