package scheduler_test

import (
	"fmt"
	"testing"
	"time"

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
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var (
	mock_definitions = []string{`
name: Server.Internal.ArtifactDescription
type: SERVER
`}
)

type MinionSchedulerTestSuite struct {
	test_utils.TestSuite
}

func (self *MinionSchedulerTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.NotebookService = true
	self.ConfigObj.Services.SchedulerService = true
	self.ConfigObj.Services.ApiServer = true
	// Do not start local workers to force us to go through the remote
	// one.
	self.ConfigObj.Defaults.NotebookNumberOfLocalWorkers = -1

	self.LoadArtifactsIntoConfig(mock_definitions)
	self.TestSuite.SetupTest()
}

func (self *MinionSchedulerTestSuite) startAPIServer() {
	builder, err := api.NewServerBuilder(self.Ctx, self.ConfigObj, self.Sm.Wg)
	assert.NoError(self.T(), err)

	err = builder.WithAPIServer(self.Ctx, self.Sm.Wg)
	assert.NoError(self.T(), err)
}

func (self *MinionSchedulerTestSuite) TestMinionScheduler() {
	self.startAPIServer()

	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	org_manager.Services("").(*orgs.ServiceContainer).MockFrontendManager(
		frontend.NewMinionFrontendManager(self.ConfigObj, ""))

	// Get a minion scheduler that will connect to the api server.
	minion_scheduler := scheduler.NewMinionScheduler(self.ConfigObj, self.Ctx)
	minion_notebook_service := notebook.NewNotebookManager(self.ConfigObj,
		notebook.NewNotebookStore(self.ConfigObj))

	// Start a worker and connect to the api
	err = minion_notebook_service.RegisterWorker(self.Ctx, self.ConfigObj, minion_scheduler)
	assert.NoError(self.T(), err)

	time.Sleep(time.Second)

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Create a notebook the usual way
	notebook, err := notebook_manager.NewNotebook(self.Ctx, "admin", &api_proto.NotebookMetadata{
		Name:        "Test Notebook",
		Description: "This is a test",
	})
	assert.NoError(self.T(), err)

	// Now update the cell to some markdown
	cell, err := notebook_manager.UpdateNotebookCell(self.Ctx, notebook,
		"admin", &api_proto.NotebookCellRequest{
			NotebookId: notebook.NotebookId,
			CellId:     notebook.CellMetadata[0].CellId,
			Input:      "/*\n# 1\n*/\nselect sleep(time=1) FROM scope()\n",
			Type:       "vql",
		})
	assert.NoError(self.T(), err)

	json.Dump(cell)

	fmt.Println("Test done!")

}

func TestMinionScheduler(t *testing.T) {
	suite.Run(t, &MinionSchedulerTestSuite{})
}
