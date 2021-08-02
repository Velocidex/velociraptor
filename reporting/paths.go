package reporting

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/timelines"
)

type NotebookPathManager struct {
	notebook_id string
	root        api.PathSpec
}

func NotebookDir() api.PathSpec {
	return api.NewSafeDatastorePath("notebooks").SetType("json")
}

func (self *NotebookPathManager) Attachment(name string) api.PathSpec {
	return self.root.AddUnsafeChild(self.notebook_id, name)
}

func (self *NotebookPathManager) Path() api.PathSpec {
	return self.root.AddChild(self.notebook_id)
}

func (self *NotebookPathManager) Cell(cell_id string) *NotebookCellPathManager {
	return &NotebookCellPathManager{
		notebook_id: self.notebook_id,
		cell_id:     cell_id,
		root:        self.root,
	}
}

func (self *NotebookPathManager) CellDirectory(cell_id string) api.PathSpec {
	return self.root.AddChild(self.notebook_id, cell_id)
}

func (self *NotebookPathManager) Directory() api.PathSpec {
	return self.root.AddChild(self.notebook_id)
}

func (self *NotebookPathManager) HtmlExport() api.PathSpec {
	return api.NewUnsafeDatastorePath("downloads", "notebooks",
		self.notebook_id,
		fmt.Sprintf("%s-%s.html", self.notebook_id,
			time.Now().Format("20060102150405Z")))
}

func (self *NotebookPathManager) ZipExport() api.PathSpec {
	return api.NewUnsafeDatastorePath("downloads", "notebooks",
		self.notebook_id,
		fmt.Sprintf("%s-%s.zip", self.notebook_id,
			time.Now().Format("20060102150405Z")))
}

func (self *NotebookPathManager) TimelineDir() api.PathSpec {
	return self.Directory().AddChild("timelines")
}

func (self *NotebookPathManager) Timeline(name string) *timelines.SuperTimelinePathManager {
	return &timelines.SuperTimelinePathManager{
		Root: self.TimelineDir(),
		Name: name,
	}
}

var notebook_regex = regexp.MustCompile(`N\.(F\.[^-]+?)-(C\..+|server)`)

func NewNotebookPathManager(notebook_id string) *NotebookPathManager {
	if strings.HasPrefix(notebook_id, "N.H.") {
		// For hunt notebooks store them in the hunt itself.
		return &NotebookPathManager{
			notebook_id: notebook_id,
			root: api.NewUnsafeDatastorePath("hunts",
				strings.TrimPrefix(notebook_id, "N."),
				"notebook").SetType("json"),
		}
	}

	matches := notebook_regex.FindStringSubmatch(notebook_id)
	if len(matches) == 3 {
		// For collections notebooks store them in the hunt itself.
		return &NotebookPathManager{
			notebook_id: notebook_id,
			root: api.NewUnsafeDatastorePath("clients", matches[2],
				"collections", matches[1], "notebook"),
		}
	}

	return &NotebookPathManager{
		notebook_id: notebook_id,
		root: api.NewUnsafeDatastorePath("notebooks").
			SetType("json"),
	}
}

type NotebookCellPathManager struct {
	notebook_id, cell_id string
	table_id             int64
	root                 api.PathSpec
}

func (self *NotebookCellPathManager) Path() api.PathSpec {
	return self.root.AddChild(self.notebook_id, self.cell_id).SetType("json")
}

func (self *NotebookCellPathManager) Notebook() *NotebookPathManager {
	return &NotebookPathManager{
		notebook_id: self.notebook_id,
		root:        self.root,
	}
}

func (self *NotebookCellPathManager) Item(name string) api.PathSpec {
	return self.root.AddChild(self.notebook_id, self.cell_id, name)
}

func (self *NotebookCellPathManager) NewQueryStorage() *NotebookCellQuery {
	self.table_id++
	return &NotebookCellQuery{
		notebook_id: self.notebook_id,
		cell_id:     self.cell_id,
		id:          self.table_id,
		root:        self.root,
	}
}

func (self *NotebookCellPathManager) QueryStorage(id int64) *NotebookCellQuery {
	return &NotebookCellQuery{
		notebook_id: self.notebook_id,
		cell_id:     self.cell_id,
		id:          id,
		root:        self.root,
	}
}

type NotebookCellQuery struct {
	notebook_id, cell_id string
	id                   int64
	root                 api.PathSpec
}

func (self *NotebookCellQuery) Path() api.PathSpec {
	return self.root.AddChild(self.notebook_id, self.cell_id,
		fmt.Sprintf("query_%d", self.id)).SetType("json")
}

func (self *NotebookCellQuery) Params() *ordereddict.Dict {
	return ordereddict.NewDict().Set("notebook_id", self.notebook_id).
		Set("cell_id", self.cell_id).
		Set("table_id", self.id)
}

type NotebookExportPathManager struct {
	notebook_id string
	root        api.PathSpec
}

func (self *NotebookExportPathManager) CellMetadata(cell_id string) api.PathSpec {
	return self.root.AddChild(self.notebook_id, cell_id)
}

func (self *NotebookExportPathManager) CellItem(cell_id, name string) api.PathSpec {
	return self.root.AddChild(self.notebook_id, cell_id, name)
}

func NewNotebookExportPathManager(notebook_id string) *NotebookExportPathManager {
	return &NotebookExportPathManager{
		notebook_id: notebook_id,
		root:        api.NewUnsafeDatastorePath("notebooks", notebook_id),
	}
}

type ContainerPathManager struct {
	artifact string
}

func (self *ContainerPathManager) Path() string {
	return self.artifact + ".json"
}

func (self *ContainerPathManager) CSVPath() string {
	return self.artifact + ".csv"
}

func NewContainerPathManager(artifact string) *ContainerPathManager {
	// Zip paths must not have leading /
	artifact = strings.TrimPrefix(artifact, "/")

	return &ContainerPathManager{artifact: artifact}
}
