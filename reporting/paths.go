package reporting

import (
	"www.velocidex.com/golang/velociraptor/accessors"
)

type NotebookExportPathManager struct {
	notebook_id string
	root        *accessors.OSPath
}

func (self *NotebookExportPathManager) CellMetadata(
	cell_id string) *accessors.OSPath {
	return self.root.Append(self.notebook_id, cell_id+".db")
}

func (self *NotebookExportPathManager) UploadRoot() *accessors.OSPath {
	return self.root.Append(self.notebook_id, "files")
}

func (self *NotebookExportPathManager) CellUploadRoot(
	cell_id string) *accessors.OSPath {
	return self.root.Append(self.notebook_id, cell_id, "uploads")
}

func (self *NotebookExportPathManager) CellDirectory(
	cell_id string) *accessors.OSPath {
	return self.root.Append(self.notebook_id, cell_id)
}

func NewNotebookExportPathManager(notebook_id string) *NotebookExportPathManager {
	root, _ := accessors.NewZipFilePath("/")

	return &NotebookExportPathManager{
		notebook_id: notebook_id,
		root:        root,
	}
}
