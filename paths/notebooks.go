package paths

import (
	"fmt"
	"regexp"
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

// Where to store attachments? In the notebook path.
func (self *NotebookPathManager) Attachment(name string) api.FSPathSpec {
	return self.root.AddUnsafeChild(self.notebook_id, "files", name).
		AsFilestorePath().SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self *NotebookPathManager) AttachmentDirectory() api.FSPathSpec {
	return self.root.AddChild(self.notebook_id, "files").
		AsFilestorePath().SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self *NotebookPathManager) Path() api.DSPathSpec {
	return self.root.AddChild(self.notebook_id).SetTag("Notebook")
}

func (self *NotebookPathManager) Cell(cell_id string) *NotebookCellPathManager {
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

func (self *NotebookPathManager) HtmlExport() api.FSPathSpec {
	return DOWNLOADS_ROOT.AddChild("notebooks", self.notebook_id,
		fmt.Sprintf("%s-%s", self.notebook_id,
			self.Clock.Now().Format("20060102150405Z"))).
		SetType(api.PATH_TYPE_FILESTORE_DOWNLOAD_REPORT)
}

func (self *NotebookPathManager) ZipExport() api.FSPathSpec {
	return DOWNLOADS_ROOT.AddChild("notebooks", self.notebook_id,
		fmt.Sprintf("%s-%s", self.notebook_id,
			self.Clock.Now().Format("20060102150405Z"))).
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

// A notebook id for clients flows
var client_notebook_regex = regexp.MustCompile(`^N\.(F\.[^-]+?)-(C\..+|server)$`)
var event_notebook_regex = regexp.MustCompile(`^N\.E\.([^-]+?)-(C\..+|server)$`)

func rootPathFromNotebookID(notebook_id string) api.DSPathSpec {
	if strings.HasPrefix(notebook_id, "Dashboard") {
		return NOTEBOOK_ROOT.AddUnsafeChild("Dashboards").
			SetType(api.PATH_TYPE_DATASTORE_JSON)
	}

	if strings.HasPrefix(notebook_id, "N.H.") {
		// For hunt notebooks store them in the hunt itself.
		return HUNTS_ROOT.AddChild(
			strings.TrimPrefix(notebook_id, "N."), "notebook").
			SetType(api.PATH_TYPE_DATASTORE_JSON)
	}

	matches := client_notebook_regex.FindStringSubmatch(notebook_id)
	if len(matches) == 3 {
		// For collections notebooks store them in the hunt itself.
		return CLIENTS_ROOT.AddChild(matches[2],
			"collections", matches[1], "notebook").
			SetType(api.PATH_TYPE_DATASTORE_JSON)
	}

	matches = event_notebook_regex.FindStringSubmatch(notebook_id)
	if len(matches) == 3 {
		// For event notebooks, store them in the client's monitoring
		// area.
		return CLIENTS_ROOT.AddUnsafeChild(matches[2],
			"monitoring_notebooks", matches[1]).
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

func (self *NotebookCellPathManager) Path() api.DSPathSpec {
	return self.root.AddUnsafeChild(self.notebook_id, self.cell_id).
		SetTag("NotebookCell")
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

// Uploads are stored in each cell separately.
func (self *NotebookCellPathManager) UploadsDir() api.FSPathSpec {
	return self.root.AsFilestorePath().
		AddUnsafeChild(self.notebook_id, self.cell_id, "uploads").
		SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self *NotebookCellPathManager) GetUploadsFile(filename string) api.FSPathSpec {
	return self.root.AsFilestorePath().
		AddUnsafeChild(self.notebook_id,
			self.cell_id, "uploads", filename).
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

type ContainerPathManager struct {
	artifact string
}

func (self *ContainerPathManager) Path() string {
	return "results/" + utils.SanitizeString(self.artifact) + ".json"
}

func (self *ContainerPathManager) CSVPath() string {
	return "results/" + utils.SanitizeString(self.artifact) + ".csv"
}

func NewContainerPathManager(artifact string) *ContainerPathManager {
	return &ContainerPathManager{artifact: artifact}
}
