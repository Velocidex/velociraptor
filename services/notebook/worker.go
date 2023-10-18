package notebook

import (
	"context"
	"sync"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

type NotebookRequest struct {
	NotebookMetadata    *api_proto.NotebookMetadata
	Username            string
	NotebookCellRequest *api_proto.NotebookCellRequest
}

type NotebookResponse struct {
	NotebookCell *api_proto.NotebookCell
}

func (self *NotebookManager) processUpdateRequest(
	ctx context.Context,
	config_obj *config_proto.Config,
	request *NotebookRequest) (*NotebookResponse, error) {

	notebook_metadata := request.NotebookMetadata
	user_name := request.Username
	in := request.NotebookCellRequest

	notebook_cell := &api_proto.NotebookCell{
		Input:            in.Input,
		CellId:           in.CellId,
		Type:             in.Type,
		Timestamp:        utils.GetTime().Now().Unix(),
		CurrentlyEditing: in.CurrentlyEditing,
		Calculating:      true,
		Env:              in.Env,
	}

	notebook_path_manager := paths.NewNotebookPathManager(
		notebook_metadata.NotebookId)

	err := self.Store.SetNotebook(notebook_metadata)
	if err != nil {
		return nil, err
	}

	query_ctx, query_cancel := context.WithCancel(ctx)
	acl_manager := acl_managers.NewServerACLManager(self.config_obj, user_name)

	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		return nil, err
	}
	global_repo, err := manager.GetGlobalRepository(self.config_obj)
	if err != nil {
		return nil, err
	}

	tmpl, err := reporting.NewGuiTemplateEngine(
		self.config_obj, query_ctx, nil, acl_manager, global_repo,
		notebook_path_manager.Cell(in.CellId),
		"Server.Internal.ArtifactDescription")
	if err != nil {
		return nil, err
	}
	defer tmpl.Close()

	tmpl.SetEnv("NotebookId", in.NotebookId)

	// Register a progress reporter so we can monitor how the
	// template rendering is going.
	tmpl.Progress = &progressReporter{
		config_obj:    self.config_obj,
		notebook_cell: notebook_cell,
		notebook_id:   in.NotebookId,
		start:         utils.GetTime().Now(),
		store:         self.Store,
	}

	// Add the notebook environment into the cell template.
	for _, env := range notebook_metadata.Env {
		tmpl.SetEnv(env.Key, env.Value)
	}

	// Also apply the cell env
	for _, env := range in.Env {
		tmpl.SetEnv(env.Key, env.Value)
	}

	input := in.Input
	cell_type := in.Type

	// Update the content asynchronously
	start_time := utils.GetTime().Now()

	// Watcher thread: Wait for cancellation from the GUI or a 10 min timeout.
	go func() {
		defer query_cancel()

		notifier, err := services.GetNotifier(self.config_obj)
		if err != nil {
			return
		}
		cancel_notify, remove_notification := notifier.
			ListenForNotification(in.CellId)
		defer remove_notification()

		default_notebook_expiry := self.config_obj.Defaults.NotebookCellTimeoutMin
		if default_notebook_expiry == 0 {
			default_notebook_expiry = 10
		}

		select {
		case <-ctx.Done():

		// Active cancellation from the GUI.
		case <-cancel_notify:
			tmpl.Scope.Log("Cancelled after %v !", time.Since(start_time))

			// Set a timeout.
		case <-time.After(time.Duration(default_notebook_expiry) * time.Minute):
			tmpl.Scope.Log("Query timed out after %v !", time.Since(start_time))
		}
	}()

	resp, err := self.updateCellContents(query_ctx, tmpl,
		in.CurrentlyEditing, in.NotebookId,
		in.CellId, cell_type, in.Env, input, in.Input)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
		logger.Error("Rendering error: %v", err)
		return nil, err
	}

	return &NotebookResponse{
		NotebookCell: resp,
	}, nil
}

func (self *NotebookManager) registerWorker(
	ctx context.Context,
	config_obj *config_proto.Config) error {
	scheduler, err := services.GetSchedulerService(config_obj)
	if err != nil {
		return err
	}

	job_chan, err := scheduler.RegisterWorker(ctx, "Notebook", 10)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case job, ok := <-job_chan:
				if !ok {
					return
				}
				request := &NotebookRequest{}
				err := json.Unmarshal([]byte(job.Job), request)
				if err != nil {
					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Error("NotebookManager: Invalid job request in worker: %v: %v",
						err, job.Job)
					continue
				}

				// Call our processor
				resp, err := self.processUpdateRequest(ctx, config_obj, request)
				serialized, _ := json.Marshal(resp)
				job.Done(string(serialized), err)
			}
		}
	}()

	return nil
}

func (self *NotebookManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	// Start a few local workers for notebooks. This also limits
	// concurrency on notebook processing.
	local_workers := int64(1)
	if config_obj.Defaults.NotebookNumberOfLocalWorkers != 0 {
		local_workers = config_obj.Defaults.NotebookNumberOfLocalWorkers
	}

	for i := int64(0); i < local_workers; i++ {
		err := self.registerWorker(ctx, config_obj)
		if err != nil {
			return err
		}
	}

	return nil
}
