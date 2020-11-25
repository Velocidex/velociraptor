package reporting

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type NotebookPathManager struct {
	notebook_id string
	root        string
}

func NotebookDir() string {
	return "/notebooks/"
}

func (self *NotebookPathManager) Path() string {
	return path.Join(self.root, self.notebook_id+".json")
}

func (self *NotebookPathManager) Cell(cell_id string) *NotebookCellPathManager {
	return &NotebookCellPathManager{
		notebook_id: self.notebook_id,
		cell_id:     cell_id,
		root:        self.root,
	}
}

func (self *NotebookPathManager) CellDirectory(cell_id string) string {
	return path.Join(self.root, self.notebook_id, cell_id)
}

func (self *NotebookPathManager) HtmlExport() string {
	return path.Join("/downloads/", self.root, self.notebook_id,
		fmt.Sprintf("%s-%s.html", self.notebook_id,
			time.Now().Format("20060102150405Z")))
}

func (self *NotebookPathManager) ZipExport() string {
	return path.Join("/downloads/", self.root, self.notebook_id,
		fmt.Sprintf("%s-%s.zip", self.notebook_id,
			time.Now().Format("20060102150405Z")))
}

var notebook_regex = regexp.MustCompile(`N\.(F\.[^-]+?)-(C\..+|server)`)

func NewNotebookPathManager(notebook_id string) *NotebookPathManager {
	if strings.HasPrefix(notebook_id, "N.H.") {
		// For hunt notebooks store them in the hunt itself.
		return &NotebookPathManager{
			notebook_id: notebook_id,
			root: path.Join("/hunts",
				strings.TrimPrefix(notebook_id, "N."), "notebook"),
		}
	}

	matches := notebook_regex.FindStringSubmatch(notebook_id)
	if len(matches) == 3 {
		// For hunt notebooks store them in the hunt itself.
		return &NotebookPathManager{
			notebook_id: notebook_id,
			root: path.Join("/clients/", matches[2],
				"collections", matches[1], "notebook"),
		}
	}

	return &NotebookPathManager{
		notebook_id: notebook_id,
		root:        "notebooks",
	}
}

type NotebookCellPathManager struct {
	notebook_id, cell_id string
	table_id             int64
	root                 string
}

func (self *NotebookCellPathManager) Path() string {
	return path.Join(self.root, self.notebook_id, self.cell_id+".json")
}

func (self *NotebookCellPathManager) Item(name string) string {
	return path.Join(self.root, self.notebook_id, self.cell_id, name)
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

func (self *NotebookCellPathManager) QueryStorage(id int64) api.PathManager {
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
	root                 string
}

func (self *NotebookCellQuery) Path() string {
	return fmt.Sprintf("/%s/%s/%s/query_%d.json", self.root,
		self.notebook_id, self.cell_id, self.id)
}

func (self *NotebookCellQuery) GetPathForWriting() (string, error) {
	return self.Path(), nil
}

func (self *NotebookCellQuery) Params() *ordereddict.Dict {
	return ordereddict.NewDict().Set("notebook_id", self.notebook_id).
		Set("cell_id", self.cell_id).
		Set("table_id", self.id)
}

func (self *NotebookCellQuery) GetQueueName() string {
	return ""
}

func (self *NotebookCellQuery) GeneratePaths(ctx context.Context) <-chan *api.ResultSetFileProperties {
	output := make(chan *api.ResultSetFileProperties)

	go func() {
		defer close(output)

		output <- &api.ResultSetFileProperties{
			Path: self.Path(),
		}
	}()

	return output
}

type NotebookExportPathManager struct {
	notebook_id string
}

func (self *NotebookExportPathManager) CellMetadata(cell_id string) string {
	return "/" + self.notebook_id + "/" + cell_id
}

func (self *NotebookExportPathManager) CellItem(cell_id, name string) string {
	return "/" + self.notebook_id + "/" + cell_id + "/" + name
}

func NewNotebookExportPathManager(notebook_id string) *NotebookExportPathManager {
	return &NotebookExportPathManager{notebook_id}
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
