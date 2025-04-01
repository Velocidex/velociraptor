package notebook

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *NotebookManager) NewNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest, username string) (
	*api_proto.NotebookMetadata, error) {

	// Calculate the cell first then insert it into the notebook.
	new_version := GetNextVersion("")
	new_cell_request := &api_proto.NotebookCellRequest{
		Input:             in.Input,
		Output:            in.Output,
		CellId:            NewNotebookCellId(),
		NotebookId:        in.NotebookId,
		Version:           new_version,
		AvailableVersions: []string{new_version},
		Type:              in.Type,
		Env:               in.Env,
		Sync:              in.Sync,

		// New cells are opened for editing.
		CurrentlyEditing: true,
	}

	// TODO: This is not thread safe!
	notebook, err := self.Store.GetNotebook(in.NotebookId)
	if err != nil {
		return nil, err
	}

	// Start off with some empty lines.
	if in.Input == "" {
		in.Input = "\n\n\n\n\n\n"
	}

	new_cell, err := self.UpdateNotebookCell(
		ctx, notebook, username, new_cell_request)
	if err != nil {
		return nil, err
	}

	// The notebook only keep summary metadata and not the full
	// results.
	new_cell_summary := &api_proto.NotebookCell{
		CellId:            new_cell.CellId,
		CurrentVersion:    new_cell.CurrentVersion,
		AvailableVersions: new_cell.AvailableVersions,
		Timestamp:         new_cell.Timestamp,
	}

	added := false
	now := utils.GetTime().Now().UnixNano()

	new_cell_md := []*api_proto.NotebookCell{}

	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == in.CellId {

			// New cell goes above existing cell.
			new_cell_md = append(new_cell_md, new_cell_summary)

			cell_md.Timestamp = now
			new_cell_md = append(new_cell_md, cell_md)
			added = true
			continue
		}
		new_cell_md = append(new_cell_md, cell_md)
	}

	// Add it to the end of the document.
	if !added {
		new_cell_md = append(new_cell_md, new_cell_summary)
	}

	notebook.LatestCellId = new_cell.CellId
	notebook.CellMetadata = new_cell_md
	notebook.ModifiedTime = new_cell.Timestamp

	err = self.Store.SetNotebook(notebook)
	if err != nil {
		return nil, err
	}

	return notebook, err
}
