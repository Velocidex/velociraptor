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

	// Write the cell record as calculating while we attempt to
	// schedule it.
	notebook_cell := &api_proto.NotebookCell{
		Input:            in.Input,
		CellId:           in.CellId,
		Type:             in.Type,
		Timestamp:        time.Now().Unix(),
		CurrentlyEditing: in.CurrentlyEditing,
		Calculating:      true,
		Env:              in.Env,
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
