package notebooks

import (
	"archive/zip"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers"
)

var testArtifacts = []string{`
name: Test.Artifact
type: NOTEBOOK
sources:
- notebook:
  - type: markdown
    template: "# Hello world"
  - type: vql_suggestion
    name: A Cell suggestion
    template: "This is a suggestion"
`, `
name: Server.Audit.Logs
type: SERVER_EVENT
`, `
name: Server.Internal.ArtifactDescription
`}

type NotebookTestSuite struct {
	test_utils.TestSuite
	acl_manager vql_subsystem.ACLManager

	closer func()
}

func (self *NotebookTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.HuntDispatcher = true
	self.ConfigObj.Services.NotebookService = true
	self.ConfigObj.Services.SchedulerService = true

	self.TestSuite.SetupTest()

	// Create an administrator user
	err := services.GrantRoles(self.ConfigObj, "admin", []string{"administrator"})
	assert.NoError(self.T(), err)

	self.acl_manager = acl_managers.NewServerACLManager(
		self.ConfigObj, "admin")

	gen := utils.IncrementalIdGenerator(0)
	self.closer = utils.SetIdGenerator(&gen)
}

func (self *NotebookTestSuite) TearDownTest() {
	self.closer()
	self.TestSuite.TearDownTest()
}

func (self *NotebookTestSuite) TestCreateNotebook() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1602103388, 0)))
	defer closer()

	repository := self.LoadArtifacts(testArtifacts...)
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: self.acl_manager,
		Repository: repository,
		Logger: logging.NewPlainLogger(
			self.ConfigObj, &logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)
	scope := manager.BuildScope(builder)
	defer scope.Close()

	var notebook *api_proto.NotebookMetadata

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		var ok bool

		plugin := &CreateNotebookFunction{}
		res_any := plugin.Call(self.Ctx, scope,
			ordereddict.NewDict().
				Set("name", "TestNotebook").
				Set("description", "A test notebook").
				Set("collaborators", []string{"mic", "fred"}).
				Set("artifacts", "Test.Artifact"))

		notebook, ok = res_any.(*api_proto.NotebookMetadata)
		return ok
	})

	// One cell created which contains Hello world.
	assert.Equal(self.T(), len(notebook.CellMetadata), 1)

	// Make sure the cell is rendered.
	assert.Contains(self.T(), notebook.CellMetadata[0].Output, "Hello world")

	// With one suggestion added from the NOTEBOOK artifact
	assert.Equal(self.T(), len(notebook.Suggestions), 1)
	assert.Equal(self.T(), notebook.Suggestions[0].Input,
		"This is a suggestion")

	// Add an attachment to the notebook
	UpdateNotebookFunction{}.Call(self.Ctx, scope,
		ordereddict.NewDict().
			Set("notebook_id", notebook.NotebookId).
			Set("attachment", "Hello world").
			Set("attachment_filename", "attachment.txt"))

	// Now update the cell with new content.
	res_any := UpdateNotebookCellFunction{}.Call(self.Ctx, scope,
		ordereddict.NewDict().
			Set("notebook_id", notebook.NotebookId).
			Set("cell_id", notebook.CellMetadata[0].CellId).
			Set("type", "vql").
			Set("input", "SELECT 1 AS A, 2 AS B, upload(accessor='data', file='hello', name='file.txt') AS Upload FROM scope()"))

	notebook = res_any.(*api_proto.NotebookMetadata)

	// This will contain the HTML output of the rendered query.
	assert.Contains(self.T(),
		notebook.CellMetadata[0].Output, "velo-csv-viewer")

	// Now there are two versions of this cell.
	assert.Equal(self.T(),
		len(notebook.CellMetadata[0].AvailableVersions), 2)

	// Update a cell with no cell id means to add a new cell. This
	// time we also specify the output which should cause the output
	// to be written and not calculated. We add a link to the
	// attachment we added before to simulate the GUI pasting an image
	// into the cell.
	res_any = UpdateNotebookCellFunction{}.Call(self.Ctx, scope,
		ordereddict.NewDict().
			Set("notebook_id", notebook.NotebookId).
			Set("input", "# Input field").
			// Two types of links will be converted: img and a links.
			Set("output", "Output Field With <img src=\"/notebooks/N.01/attach/NA.04-attachment.txt?org_id=root\">\n\n<a href=\"/notebooks/N.01/attach/NA.04-attachment.txt?org_id=root\">file</a> "))

	notebook = res_any.(*api_proto.NotebookMetadata)
	assert.Equal(self.T(), len(notebook.CellMetadata), 2)
	assert.Contains(self.T(),
		notebook.CellMetadata[1].Input, "Input")

	// Output is not modified.
	assert.Contains(self.T(),
		notebook.CellMetadata[1].Output, "Output")

	// The last cell was added at the end.
	assert.Equal(self.T(), notebook.LatestCellId, notebook.CellMetadata[1].CellId)

	// Check auditing and logging
	mem_file_store := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)

	// Wait for the audit messages to be written
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		audit, _ := mem_file_store.Get("/server_artifacts/Server.Audit.Logs/2020-10-07.json")
		return strings.Contains(string(audit), `"Input":"# Input field"`) &&
			strings.Contains(string(audit),
				`"operation":"CreateNotebook","principal":"admin"`) &&
			strings.Contains(string(audit),
				`"operation":"UpdateNotebookCell","principal":"admin"`)
	})

	// Check uploads - uploads are stored in each cell so they can be versioned
	// mem_file_store.Debug()

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		upload, pres := mem_file_store.Get(
			"/notebooks/N.01/NC.02-05/uploads/data/file.txt")
		if !pres {
			return false
		}
		assert.Contains(self.T(), string(upload), `hello`)
		return len(upload) > 0
	})

	// Attachments are global to the whole notebook and are not
	// versioned.
	attachment, _ := mem_file_store.Get(
		"/notebooks/N.01/attach/NA.04-attachment.txt")
	assert.Equal(self.T(), string(attachment), "Hello world")

	// Export the notebook to html.
	res_any = ExportNotebookFunction{}.Call(self.Ctx, scope,
		ordereddict.NewDict().
			Set("notebook_id", notebook.NotebookId).
			Set("type", "zip").
			Set("filename", "Test"))

	// Uncomment this to debug the test.
	// mem_file_store.Debug()

	export_path := res_any.(api.FSPathSpec)
	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	fd, err := file_store_factory.ReadFile(export_path)
	assert.NoError(self.T(), err)

	stats, err := fd.Stat()
	assert.NoError(self.T(), err)

	zip, err := zip.NewReader(
		utils.MakeReaderAtter(fd), stats.Size())
	assert.NoError(self.T(), err)

	files := ordereddict.NewDict()
	for _, f := range zip.File {
		member, err := f.Open()
		assert.NoError(self.T(), err)
		data, err := ioutil.ReadAll(member)
		assert.NoError(self.T(), err)
		files.Set(f.Name, strings.Split(string(data), "\n"))
	}

	// Uncomment to debug zip file output
	// json.Dump(files)

	// The query output is emitted (without the version since we
	// export the current version only).
	data, _ := files.GetStrings("N.01/NC.02/query_1.json")
	assert.Contains(self.T(), data[0], "{\"A\":1,\"B\":2,\"Upload\"")

	// Make sure logs are exported
	data, _ = files.GetStrings("N.01/NC.02/logs.json")
	assert.Contains(self.T(), data[0], "Uploaded /file.txt")

	// Check the upload is emitted
	data, _ = files.GetStrings("N.01/NC.02/uploads/data/file.txt")
	assert.Contains(self.T(), data[0], "hello")

	// Attachments are also exported.
	data, _ = files.GetStrings("N.01/attach/NA.04-attachment.txt")
	assert.Contains(self.T(), data[0], "Hello world")

	// Now export the notebook to html
	// Export the notebook to html.
	res_any = ExportNotebookFunction{}.Call(self.Ctx, scope,
		ordereddict.NewDict().
			Set("notebook_id", notebook.NotebookId).
			Set("type", "html").
			Set("filename", "Test")) // The html extension is added automatically.

	// Make sure the stats record for the export is written.
	mem_data_store := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)

	stat, _ := mem_data_store.GetForTests(self.ConfigObj,
		"/downloads/notebooks/N.01/Test")
	assert.Contains(self.T(), string(stat), "\"hash\"")
	assert.Contains(self.T(), string(stat), "\"Test.html\"")

	html_export_bytes, _ := mem_file_store.Get("/downloads/notebooks/N.01/Test.html")
	html_export := string(html_export_bytes)
	// Table is exported properly.
	assert.Contains(self.T(), html_export, "<th>A</th>")
	assert.Contains(self.T(), html_export, "<th>B</th>")
	assert.Contains(self.T(), html_export, "<td>1</td>")

	// Data is properly html escaped
	assert.Contains(self.T(), html_export, "<td>{&#34;Path&#34;")

	// Img tags are converted
	assert.Contains(self.T(), html_export,
		`<img src="data:image/png;base64,SGVsbG8gd29ybGQ=">`)

	// Non image attachments are converted
	assert.Contains(self.T(), html_export,
		`<a href="#" onclick="downloadFile('SGVsbG8gd29ybGQ=', 'file')">file</a>`)

	// Make sure uploads are properly encoded into the file
	assert.Contains(self.T(), html_export, "downloadFile('aGVsbG8=', 'file.txt')")
}

func TestNotebook(t *testing.T) {
	suite.Run(t, &NotebookTestSuite{})
}
