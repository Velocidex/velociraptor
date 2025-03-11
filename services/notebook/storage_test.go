package notebook_test

import (
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/scheduler"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *NotebookManagerTestSuite) TestNotebookStorage() {

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)

	// Create a flow to hold the flow notebook
	closer := utils.SetFlowIdForTests("F.1234")
	acl_manager := acl_managers.NewServerACLManager(self.ConfigObj, "admin")
	_, err = launcher.ScheduleArtifactCollection(
		self.Ctx, self.ConfigObj, acl_manager,
		repository, &flows_proto.ArtifactCollectorArgs{
			Creator:   "admin",
			ClientId:  "C.1235",
			Artifacts: []string{"Generic.Client.Info"},
		}, nil)
	assert.NoError(self.T(), err)

	closer()

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	notebooks, err := notebook_manager.GetAllNotebooks(self.Ctx,
		services.NotebookSearchOptions{})
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 0, len(notebooks))

	scheduler_service, err := services.GetSchedulerService(self.ConfigObj)
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		return scheduler_service.(*scheduler.Scheduler).AvailableWorkers() > 0
	})

	var global_notebook *api_proto.NotebookMetadata

	// Create a notebook the usual way.
	global_notebook, err = notebook_manager.NewNotebook(
		self.Ctx, "admin", &api_proto.NotebookMetadata{
			Name: "Test Global Notebook",
		})
	assert.NoError(self.T(), err)

	// Now create a flow notebook - these have pre-determined ID
	_, err = notebook_manager.NewNotebook(
		self.Ctx, "admin", &api_proto.NotebookMetadata{
			Name:       "Test Flow Notebook",
			NotebookId: "N.F.1234-C.1235",
		})
	assert.NoError(self.T(), err)

	// Now list all notebooks - should only see the global one.
	notebooks, err = notebook_manager.GetAllNotebooks(self.Ctx,
		services.NotebookSearchOptions{})
	assert.NoError(self.T(), err)

	if len(notebooks) != 1 {
		json.Dump(notebooks)
	}

	assert.Equal(self.T(), 1, len(notebooks))
	assert.Equal(self.T(), global_notebook.NotebookId, notebooks[0].NotebookId)
}
