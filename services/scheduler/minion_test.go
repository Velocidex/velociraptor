package scheduler_test

import (
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/services/scheduler"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/velociraptor/vql/parsers"
)

var (
	mock_definitions = []string{`
name: Server.Internal.ArtifactDescription
type: SERVER
`, `
name: Notebooks.Default
type: NOTEBOOK
sources:
- notebook:
  - type: markdown
    template: |
      # Welcome to Velociraptor notebooks!
`}
)

type MinionSchedulerTestSuite struct {
	test_utils.TestSuite
	closer func()
}

func (self *MinionSchedulerTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.NotebookService = true
	self.ConfigObj.Services.SchedulerService = true
	self.ConfigObj.Services.ApiServer = true
	// Do not start local workers to force us to go through the remote
	// one.
	self.ConfigObj.Defaults.NotebookNumberOfLocalWorkers = -1
	self.ConfigObj.Defaults.NotebookWaitTimeForWorkerMs = -1
	self.ConfigObj.API.BindPort = 8345

	// Mock out cell ID generation for tests
	gen := utils.ConstantIdGenerator("XXX")
	self.closer = utils.SetIdGenerator(gen)

	self.LoadArtifactsIntoConfig(mock_definitions)
	self.TestSuite.SetupTest()
}

func (self *MinionSchedulerTestSuite) TearDownTest() {
	self.closer()
	self.TestSuite.TearDownTest()
}

func (self *MinionSchedulerTestSuite) startAPIServer() {
	builder, err := api.NewServerBuilder(self.Ctx, self.ConfigObj, self.Sm.Wg)
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		err := builder.WithAPIServer(self.Ctx, self.Sm.Wg)
		return err == nil
	})
}

func (self *MinionSchedulerTestSuite) TestNotebookMinionScheduler() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer closer()

	self.startAPIServer()

	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	org_manager.Services(services.ROOT_ORG_ID).(*orgs.ServiceContainer).MockFrontendManager(
		frontend.NewMinionFrontendManager(self.ConfigObj, ""))

	// Get a minion scheduler that will connect to the api server.
	minion_scheduler := scheduler.NewMinionScheduler(self.ConfigObj, self.Ctx)

	// Start a worker and connect to the api. The worker remains
	// running in the background throughout.
	go func() {
		minion_worker := &notebook.NotebookWorker{}
		minion_worker.RegisterWorker(self.Ctx, self.ConfigObj,
			"Test", minion_scheduler)
	}()

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Create a notebook the usual way (This calls the worker for the
	// initial cell)
	var notebook *api_proto.NotebookMetadata
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		notebook, err = notebook_manager.NewNotebook(
			self.Ctx, "admin", &api_proto.NotebookMetadata{
				Name:        "Test Notebook",
				Description: "This is a test",
			})
		return err == nil
	})

	cell_id := notebook.CellMetadata[0].CellId

	// Now update the cell to some markdown
	cell, err := notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
		"admin", &api_proto.NotebookCellRequest{
			NotebookId: notebook.NotebookId,
			CellId:     cell_id,
			Input:      "/*\n# 1\n*/\nselect sleep(ms=500) FROM scope()\n",
			Type:       "vql",
		})
	assert.NoError(self.T(), err)

	cell.Timestamp = 0
	golden := ordereddict.NewDict().
		Set("Updated Cell", cell)

	var wg sync.WaitGroup

	// Test cancellations
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Should block for 500 sec if cancellation fails.
		cell, err := notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
			"admin", &api_proto.NotebookCellRequest{
				NotebookId: notebook.NotebookId,
				CellId:     cell_id,
				Input:      "/*\n# 1\n*/\nselect sleep(time=500) FROM scope()\n",
				Type:       "vql",
			})
		assert.NoError(self.T(), err)
		assert.Contains(self.T(), json.MustMarshalString(cell), "Cancelled")
	}()

	// Issue the cancellaion. Cancellation should be dispatched across
	// to the minion through the notification service.
	time.Sleep(200 * time.Millisecond)

	// Trying to schedule now should result in an error because all
	// workers are busy.
	cell, err = notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
		"admin", &api_proto.NotebookCellRequest{
			NotebookId: notebook.NotebookId,
			CellId:     cell_id,
			Input:      "/*\n# 1\n*/\nselect sleep(time=500) FROM scope()\n",
			Type:       "vql",
		})
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "No workers available")

	err = notebook_manager.CancelNotebookCell(
		self.Ctx, notebook.NotebookId, cell_id, cell.CurrentVersion)
	assert.NoError(self.T(), err)

	// Wait here until the cell is cancelled
	wg.Wait()

	// Check the cell contents
	cell, err = notebook_manager.GetNotebookCell(
		self.Ctx, notebook.NotebookId, cell_id, cell.CurrentVersion)
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestNotebookMinionScheduler",
		json.MustMarshalIndent(golden))
}

func TestMinionScheduler(t *testing.T) {
	suite.Run(t, &MinionSchedulerTestSuite{})
}
