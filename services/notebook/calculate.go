package notebook

import (
	"context"
	"errors"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *NotebookManager) UpdateNotebookCell(
	ctx context.Context,
	notebook_metadata *api_proto.NotebookMetadata,
	user_name string,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	// Request does not specify a version - get the current version
	// from the data store and append a new version on the end.
	if in.Version == "" {
		cell_metadata, err := self.getCurrentCellVersion(
			in.NotebookId, in.CellId)
		if err != nil {
			return nil, err
		}

		// The cell is currently not versioned at all - treat the old
		// version as an empty string version. This supports backwards
		// compatibility with older releases.
		if cell_metadata.CurrentVersion == "" {
			cell_metadata.AvailableVersions = []string{""}
		}

		// We are about to add a new version on the cell, so we need
		// to trim any redo versions which will be lost once the cell
		// is recalculated.
		cell_metadata.AvailableVersions, err = self.trimRedoVersions(
			ctx, self.config_obj, notebook_metadata.NotebookId, cell_metadata.CellId,
			cell_metadata.CurrentVersion, cell_metadata.AvailableVersions)
		if err != nil {
			return nil, err
		}

		// Next version
		in.Version = GetNextVersion(cell_metadata.CurrentVersion)
		in.AvailableVersions = append(cell_metadata.AvailableVersions, in.Version)

		// Trim older versions
		in.AvailableVersions, err = self.trimOldCellVersions(
			ctx, self.config_obj, notebook_metadata.NotebookId,
			in.CellId, in.AvailableVersions)
		if err != nil {
			return nil, err
		}
	}

	// Write the cell record as calculating while we attempt to
	// schedule it.
	notebook_cell := &api_proto.NotebookCell{
		Input:             in.Input,
		CellId:            in.CellId,
		Type:              in.Type,
		Timestamp:         time.Now().Unix(),
		CurrentlyEditing:  in.CurrentlyEditing,
		Calculating:       true,
		Env:               in.Env,
		CurrentVersion:    in.Version,
		AvailableVersions: in.AvailableVersions,
	}

	err := self.Store.SetNotebookCell(
		notebook_metadata.NotebookId, notebook_cell)
	if err != nil {
		return nil, err
	}

	request := &NotebookRequest{
		NotebookMetadata:    notebook_metadata,
		Username:            user_name,
		NotebookCellRequest: in,
	}

	scheduler, err := services.GetSchedulerService(self.config_obj)
	if err != nil {
		return nil, err
	}

	response_chan, err := scheduler.Schedule(ctx, services.SchedulerJob{
		Queue: "Notebook",
		Job:   json.MustMarshalString(request),
		OrgId: self.config_obj.OrgId,
	})
	if err != nil {
		// Failed to schedule this cell - write the error in the cell.
		notebook_cell.Calculating = false
		notebook_cell.Error = err.Error()
		notebook_cell.Messages = append(notebook_cell.Messages,
			"ERROR:"+err.Error())

		self.Store.SetNotebookCell(notebook_metadata.NotebookId, notebook_cell)

		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, errors.New("Cancelled")

	case job_resp, ok := <-response_chan:
		if !ok {
			return nil, errors.New("Cancelled")
		}
		notebook_resp := &NotebookResponse{}
		err := json.Unmarshal([]byte(job_resp.Job), notebook_resp)
		if err != nil {
			return nil, err
		}

		return notebook_resp.NotebookCell, job_resp.Err
	}
}
