package notebook

import (
	"context"
	"errors"
	"runtime"
	"runtime/debug"
	"time"

	"www.velocidex.com/golang/vfilter/types"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/vfilter"
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

func (self *NotebookManager) startNanny(
	ctx context.Context, config_obj *config_proto.Config,
	scope vfilter.Scope,
	notebook_id, cell_id string) {

	// Reduce memory use now so the next measure of memory use is more
	// reflective of our current workload.
	debug.FreeOSMemory()

	// Running in a goroutine it's ok to block.
	for {

		// Check for high memory use.
		if self.config_obj.Defaults != nil &&
			self.config_obj.Defaults.NotebookMemoryHighWaterMark > 0 {

			high_memory_level := self.config_obj.Defaults.NotebookMemoryHighWaterMark

			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			// If we exceed memory we cancel this query.

			if high_memory_level < m.Alloc {
				scope.Log("ERROR:Insufficient resourcs: Query cancelled.")
				self.CancelNotebookCell(ctx, notebook_id, cell_id)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}

		// Check the cell for cancellation or errors
		notebook_cell, err := self.Store.GetNotebookCell(notebook_id, cell_id)
		if err != nil || notebook_cell.CellId != cell_id {
			continue
		}

		// Cancel the query - this cell is not longer running
		if !notebook_cell.Calculating {
			// Notify the calculator immediately
			notifier, err := services.GetNotifier(self.config_obj)
			if err != nil {
				return
			}
			scope.Log("ERROR:NotebookManager: Detected cell %v is cancelled. Stopping.", cell_id)
			notifier.NotifyDirectListener(cell_id)
		}
	}
}

func waitForMemoryLimit(
	ctx context.Context, scope types.Scope, config_obj *config_proto.Config) {
	// Wait until memory is below the low water mark.
	if config_obj.Defaults != nil &&
		config_obj.Defaults.NotebookMemoryLowWaterMark > 0 {

		low_memory_level := config_obj.Defaults.NotebookMemoryLowWaterMark

		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			// Spin here until there is enough memory
			if low_memory_level > m.Alloc {
				break
			}

			select {
			case <-ctx.Done():
				scope.Log("ERROR:Unable to start query before timeout - insufficient resourcs.")
				return
			case <-time.After(time.Second):
			}
		}
	}
}
