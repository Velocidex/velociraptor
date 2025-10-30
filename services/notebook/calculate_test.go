package notebook_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/notebook"
	"www.velocidex.com/golang/velociraptor/services/scheduler"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/rand"
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
name: Server.Internal.Alerts
type: SERVER_EVENT
`, `
name: Notebook.With.Parameters
type: NOTEBOOK
parameters:
- name: Bool
  type: bool
- name: StringArg
  type: string
  default: "This is a test"

column_types:
  - name: TypedColumn
    type: integer

# This will actually get this URL below.
tools:
- name: SomeTool
  url: https://www.google.com/
  serve_locally: true

sources:
- notebook:
    - type: vql
      template: |
         SELECT log(message="StringArg Should be Hello because default is overriden %v", args=StringArg),
                log(message="Tool is available through local url %v", args=Tool_SomeTool_URL)
         FROM scope()
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

type NotebookManagerTestSuite struct {
	test_utils.TestSuite
	closer func()

	client_id string
}

func (self *NotebookManagerTestSuite) SetupTest() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.NotebookService = true
	self.ConfigObj.Services.SchedulerService = true
	self.ConfigObj.Services.ClientMonitoring = true
	self.ConfigObj.Services.HuntDispatcher = true

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

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)
	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		&actions_proto.ClientInfo{
			ClientId: self.client_id,
		}})
	assert.NoError(self.T(), err)
}

func (self *NotebookManagerTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	self.closer()
}

func (self *NotebookManagerTestSuite) TestNotebookManagerUpdateCell() {
	assert.Retry(self.T(), 5, time.Second, self._TestNotebookManagerUpdateCell)
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

	result := normalizeOutput(json.MustMarshalIndent(golden))

	goldie.Retry(r, self.T(), "TestNotebookManagerUpdateCell", result)
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

// Test that notebooks can be initialized from a template and that
// tools work.
func (self *NotebookManagerTestSuite) TestNotebookFromTemplate() {
	assert.Retry(self.T(), 5, time.Second, self._TestNotebookFromTemplate)
}

func (self *NotebookManagerTestSuite) _TestNotebookFromTemplate(r *assert.R) {
	gen := utils.IncrementalIdGenerator(0)
	closer := utils.SetIdGenerator(&gen)
	defer closer()

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Create a notebook the usual way.
	var notebook *api_proto.NotebookMetadata

	notebook_metadata := &api_proto.NotebookMetadata{
		Name:        "Test Notebook",
		Description: "From Template",
		Artifacts: []string{
			"Notebook.With.Parameters",
		},
		Specs: []*flows_proto.ArtifactSpec{
			{
				Artifact: "Notebook.With.Parameters",
				Parameters: &flows_proto.ArtifactParameters{
					Env: []*actions_proto.VQLEnv{
						{Key: "StringArg", Value: "Hello"},
					},
				},
			},
		},
	}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		notebook, err = notebook_manager.NewNotebook(
			self.Ctx, "admin", notebook_metadata)
		return err == nil
	})

	assert.Equal(self.T(), len(notebook.CellMetadata), 1)

	cell, err := notebook_manager.GetNotebookCell(
		self.Ctx, notebook.NotebookId,
		notebook.CellMetadata[0].CellId,
		notebook.CellMetadata[0].CurrentVersion)
	assert.NoError(self.T(), err)

	// Clear some env that tend to change
	for _, e := range notebook.Requests[0].Env {
		if e.Key == "Tool_SomeTool_HASH" {
			e.Value = "XXXX"
		}
	}

	golden := ordereddict.NewDict().
		Set("Notebook", notebook).
		Set("Cell", cell)

	// Now update the parameters in the notebook.
	new_notebook_request := proto.Clone(
		notebook).(*api_proto.NotebookMetadata)
	new_notebook_request.Specs[0].Parameters.Env[0].Value = "Goodbye"

	err = notebook_manager.UpdateNotebook(
		self.Ctx, new_notebook_request)
	assert.NoError(self.T(), err)

	updated_notebook, err := notebook_manager.GetNotebook(self.Ctx,
		notebook.NotebookId, services.DO_NOT_INCLUDE_UPLOADS)
	assert.NoError(self.T(), err)

	// Clear some env that tend to change
	for _, e := range updated_notebook.Requests[0].Env {
		if e.Key == "Tool_SomeTool_HASH" {
			e.Value = "XXXX"
		}
	}

	golden.Set("UpdatedNotebook", updated_notebook)

	// Cell must be recalculated to pick up new env
	updated_cell, err := notebook_manager.UpdateNotebookCell(
		self.Ctx, updated_notebook, "admin",
		&api_proto.NotebookCellRequest{
			NotebookId: updated_notebook.NotebookId,
			CellId:     updated_notebook.CellMetadata[0].CellId,
			Input:      "SELECT log(message='StringArg should be Goodbye now: %v', args=StringArg) FROM scope()",
			Type:       cell.Type,
		})
	assert.NoError(self.T(), err)

	golden.Set("UpdatedCell", updated_cell)

	// Remove things that may change
	result := normalizeOutput(json.MustMarshalIndent(golden))
	goldie.Retry(r, self.T(), "TestNotebookFromTemplate", result)
}

func normalizeOutput(golden []byte) []byte {
	result := goldie.RemoveLines("timestamp", golden)
	result = goldie.RemoveLines("created_time", result)
	result = goldie.RemoveLines("modified_time", result)
	result = goldie.ReplaceLines("Version%22%3A.+%7D", "Version%22%3A%7D", result)

	return []byte(result)
}

func (self *NotebookManagerTestSuite) TestNotebookDeletion() {
	gen := utils.IncrementalIdGenerator(0)
	closer := utils.SetIdGenerator(&gen)
	defer closer()

	notebook_manager, err := services.GetNotebookManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	_, err = repository.LoadYaml(`
name: ATempArtifact
type: NOTEBOOK
parameters:
- name: FirstArg
  default: Hello

sources:
- notebook:
  - type: md
    template: "Hello world"
`, services.ArtifactOptions{ValidateArtifact: true})

	var notebook *api_proto.NotebookMetadata

	req := &api_proto.NotebookMetadata{
		Name:      "Test Notebook",
		Artifacts: []string{"ATempArtifact"},
	}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		notebook, err = notebook_manager.NewNotebook(
			self.Ctx, "admin", req)
		return err == nil
	})

	assert.Equal(self.T(), notebook.Name, "Test Notebook")

	// Now delete the initial artifact.
	err = manager.DeleteArtifactFile(self.Ctx, self.ConfigObj,
		"admin", "ATempArtifact")
	assert.NoError(self.T(), err)

	// Can not create a notebook any more - the artifact is deleted.
	_, err = notebook_manager.NewNotebook(
		self.Ctx, "admin", req)
	assert.Error(self.T(), err, "Artifact not found")

	// Modify the notebook: we should still be able to modify the
	// notebook, even though its artifact is gone because we cached
	// the artifact inside the notebook space.
	new_notebook_request := proto.Clone(
		notebook).(*api_proto.NotebookMetadata)
	new_notebook_request.Name = "An expired notebook"

	err = notebook_manager.UpdateNotebook(
		self.Ctx, new_notebook_request)
	assert.NoError(self.T(), err)

	updated_notebook, err := notebook_manager.GetNotebook(self.Ctx,
		notebook.NotebookId, services.DO_NOT_INCLUDE_UPLOADS)
	assert.NoError(self.T(), err)

	// The notebook is now updated.
	assert.Equal(self.T(), updated_notebook.Name, "An expired notebook")
}

func TestNotebookManager(t *testing.T) {
	defer rand.DisableRand()
	suite.Run(t, &NotebookManagerTestSuite{
		client_id: "C.1235",
	})
}
