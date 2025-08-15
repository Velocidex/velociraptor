package paths

import (
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

type NotebookPathManager struct {
	notebook_id string
	client_id   string
	root        api.DSPathSpec
	Clock       utils.Clock
}

func NotebookDir() api.DSPathSpec {
	return NOTEBOOK_ROOT
}

func (self *NotebookPathManager) NotebookId() string {
	return self.notebook_id
}

// Attachments are not the same as uploads - they are usually uploaded
// by pasting in the cell eg an image. We want the attachment to
// remain whenever the cell is updated to a new version.
// Example workflow:
//   - User uploads an attachment into a cell
//   - Cell is updated with a link to the attachment
//   - User continues to edit the cell in newer versions but the link
//     remains valid because the attachment is stored in the notebook
//     and not the cell.
func (self *NotebookPathManager) Attachment(name string) api.FSPathSpec {
	return self.root.AddUnsafeChild(self.notebook_id, "attach", name).
		AsFilestorePath().SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self *NotebookPathManager) AttachmentDirectory() api.FSPathSpec {
	return self.root.AddChild(self.notebook_id, "attach").
		AsFilestorePath().SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self *NotebookPathManager) NotebookDirectory() api.FSPathSpec {
	return self.root.AddChild(self.notebook_id).
		AsFilestorePath().SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self *NotebookPathManager) NotebookIndexForUser(
	username string) api.FSPathSpec {
	return self.root.AddChild("indexes", username).
		AsFilestorePath().SetType(api.PATH_TYPE_FILESTORE_JSON)
}

// Notebook paths are based on the time so we need to write the stats
// next to the container and derive the path from the previous
// filename.
func (self *NotebookPathManager) PathStats(
	filename api.FSPathSpec) api.DSPathSpec {
	return filename.AsDatastorePath().SetTag("ExportStats")
}

func (self *NotebookPathManager) Path() api.DSPathSpec {
	return self.root.AddChild(self.notebook_id).SetTag("Notebook")
}

// Stored the cached copy of the psuedo artifact we calculated when
// the artifact was first created. This is used even if the original
// artifact is deleted or modified.
func (self *NotebookPathManager) Artifact() api.DSPathSpec {
	return self.root.AddChild(self.notebook_id, "artifact").
		SetTag("Artifact")
}

// Support versioned cells by appending the version to the cell id.
func (self *NotebookPathManager) Cell(
	cell_id, version string) *NotebookCellPathManager {
	if version != "" {
		cell_id += "-" + version
	}

	return &NotebookCellPathManager{
		notebook_id: self.notebook_id,
		cell_id:     cell_id,
		client_id:   self.client_id,
		root:        self.root,
	}
}

func (self *NotebookPathManager) Directory() api.FSPathSpec {
	return self.root.AddChild(self.notebook_id).AsFilestorePath()
}

func (self *NotebookPathManager) DSDirectory() api.DSPathSpec {
	return self.root.AddChild(self.notebook_id)
}

func (self *NotebookPathManager) HtmlExport(prefered_name string) api.FSPathSpec {
	if prefered_name == "" {
		prefered_name = fmt.Sprintf("%s-%s", self.notebook_id,
			self.Clock.Now().UTC().Format("20060102150405Z"))
	}
	return DOWNLOADS_ROOT.AddChild(
		"notebooks", self.notebook_id, prefered_name).
		SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_REPORT)
}

func (self *NotebookPathManager) ZipExport() api.FSPathSpec {
	return DOWNLOADS_ROOT.AddChild("notebooks", self.notebook_id,
		fmt.Sprintf("%s-%s", self.notebook_id,
			self.Clock.Now().UTC().Format("20060102150405Z"))).
		SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP)
}

// Where we store all our super timelines
func (self *NotebookPathManager) SuperTimelineDir() api.DSPathSpec {
	return self.root.AddChild(self.notebook_id, "timelines")
}

// Create a new supertimeline in this notebook.
func (self *NotebookPathManager) SuperTimeline(
	name string) *SuperTimelinePathManager {
	return &SuperTimelinePathManager{
		Root: self.SuperTimelineDir(),
		Name: name,
	}
}

func rootPathFromNotebookID(notebook_id string) api.DSPathSpec {
	if utils.DashboardNotebookId(notebook_id) {
		return NOTEBOOK_ROOT.AddUnsafeChild("Dashboards").
			SetType(api.PATH_TYPE_DATASTORE_JSON)
	}

	hunt_id, ok := utils.HuntNotebookId(notebook_id)
	if ok {
		// For hunt notebooks store them in the hunt itself.
		return HUNTS_ROOT.AddChild(hunt_id, "notebook").
			SetType(api.PATH_TYPE_DATASTORE_JSON)
	}

	flow_id, client_id, ok := utils.ClientNotebookId(notebook_id)
	if ok {
		// For collections notebooks store them in the hunt itself.
		return CLIENTS_ROOT.AddChild(client_id,
			"collections", flow_id, "notebook").
			SetType(api.PATH_TYPE_DATASTORE_JSON)
	}

	artifact, client_id, ok := utils.EventNotebookId(notebook_id)
	if ok {
		// For event notebooks, store them in the client's monitoring
		// area.
		return CLIENTS_ROOT.AddUnsafeChild(client_id,
			"monitoring_notebooks", artifact).
			SetType(api.PATH_TYPE_DATASTORE_JSON)
	}

	return NOTEBOOK_ROOT
}

func NewNotebookPathManager(notebook_id string) *NotebookPathManager {
	return &NotebookPathManager{
		notebook_id: notebook_id,
		root:        rootPathFromNotebookID(notebook_id),
		Clock:       utils.GetTime(),
	}
}

type NotebookCellPathManager struct {
	notebook_id, cell_id string
	table_id             int64
	root                 api.DSPathSpec
	client_id            string
}

func (self *NotebookCellPathManager) Directory() api.FSPathSpec {
	return self.root.AddChild(self.notebook_id, self.cell_id).AsFilestorePath()
}

func (self *NotebookCellPathManager) DSDirectory() api.DSPathSpec {
	return self.root.AddChild(self.notebook_id, self.cell_id)
}

func (self *NotebookCellPathManager) Path() api.DSPathSpec {
	return self.root.AddUnsafeChild(self.notebook_id, self.cell_id).
		SetTag("NotebookCell")
}

func (self *NotebookCellPathManager) NotebookId() string {
	return self.notebook_id
}

func (self *NotebookCellPathManager) Notebook() *NotebookPathManager {
	return &NotebookPathManager{
		notebook_id: self.notebook_id,
		root:        self.root,
	}
}

func (self *NotebookCellPathManager) Item(name string) api.FSPathSpec {
	return self.root.AddChild(self.notebook_id, self.cell_id, name).
		AsFilestorePath()
}

func (self *NotebookCellPathManager) NewQueryStorage() *NotebookCellQuery {
	self.table_id++
	return &NotebookCellQuery{
		notebook_id: self.notebook_id,
		client_id:   self.client_id,
		cell_id:     self.cell_id,
		id:          self.table_id,
		root:        self.root.AsFilestorePath(),
	}
}

func (self *NotebookCellPathManager) Logs() api.FSPathSpec {
	return self.root.AddChild(self.notebook_id, self.cell_id, "logs").
		AsFilestorePath().SetTag("NotebookCellLogs")
}

func (self *NotebookCellPathManager) QueryStorage(id int64) *NotebookCellQuery {
	return &NotebookCellQuery{
		notebook_id: self.notebook_id,
		client_id:   self.client_id,
		cell_id:     self.cell_id,
		id:          id,
		root:        self.root.AsFilestorePath(),
	}
}

// Uploads are stored at the network level.
func (self *NotebookCellPathManager) UploadsDir() api.FSPathSpec {
	return self.root.AsFilestorePath().
		AddUnsafeChild(self.notebook_id, self.cell_id, "uploads").
		SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self *NotebookCellPathManager) GetUploadsFile(filename string) api.FSPathSpec {
	// Uploads exist inside each cell so when cells are reaped we
	// remove the uploads.
	return self.root.AsFilestorePath().
		AddUnsafeChild(self.notebook_id, self.cell_id, "uploads", filename).
		SetType(api.PATH_TYPE_FILESTORE_ANY)
}

type NotebookCellQuery struct {
	notebook_id, cell_id string
	client_id            string
	id                   int64
	root                 api.FSPathSpec
}

func (self *NotebookCellQuery) Path() api.FSPathSpec {
	return self.root.AddUnsafeChild(self.notebook_id, self.cell_id,
		fmt.Sprintf("query_%d", self.id)).
		SetTag("NotebookQuery")
}

func (self *NotebookCellQuery) Params() *ordereddict.Dict {
	result := ordereddict.NewDict().
		Set("notebook_id", self.notebook_id).
		Set("client_id", self.client_id).
		Set("cell_id", self.cell_id).
		Set("table_id", self.id)
	return result
}

// Prepare a safe string for storage in the zip file.
// Suitable escaping
func ZipPathFromFSPathSpec(path api.FSPathSpec) string {
	// Escape all components suitably for the zip file.
	components := path.Components()
	safe_components := make([]string, 0, len(components))
	for _, c := range components {
		c = utils.SanitizeStringForZip(c)
		if c == "" || c == "." || c == ".." {
			continue
		}
		safe_components = append(safe_components, c)
	}
	// Zip paths must not have a leading /
	return strings.Join(safe_components, "/") + api.GetExtensionForFilestore(path)
}
