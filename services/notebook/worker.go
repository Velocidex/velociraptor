package notebook

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/executor/throttler"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type NotebookRequest struct {
	NotebookMetadata    *api_proto.NotebookMetadata
	Username            string
	NotebookCellRequest *api_proto.NotebookCellRequest
}

type NotebookResponse struct {
	NotebookCell *api_proto.NotebookCell
}

type NotebookWorker struct{}

// Process an update request from the scheduler "Notebook" queue.
func (self *NotebookWorker) ProcessUpdateRequest(
	ctx context.Context,
	config_obj *config_proto.Config,
	request *NotebookRequest,
	store NotebookStore) (*NotebookResponse, error) {

	logger := logging.GetLogger(config_obj, &logging.GUIComponent)

	notebook_metadata := request.NotebookMetadata
	user_name := request.Username
	in := request.NotebookCellRequest

	logger.Debug("NotebookWorker: Processing cell %v/%v from %v in org %v",
		request.NotebookMetadata.NotebookId, in.CellId, request.Username,
		utils.NormalizedOrgId(config_obj.OrgId))

	// Set the cell as calculating
	notebook_cell := &api_proto.NotebookCell{
		Input:             in.Input,
		CellId:            in.CellId,
		Type:              in.Type,
		Timestamp:         utils.GetTime().Now().UnixNano(),
		CurrentlyEditing:  in.CurrentlyEditing,
		Calculating:       true,
		Output:            "Loading ...",
		Env:               in.Env,
		CurrentVersion:    in.Version,
		AvailableVersions: in.AvailableVersions,
	}

	notebook_path_manager := paths.NewNotebookPathManager(
		notebook_metadata.NotebookId)

	// The query will run in a sub context of the main context to
	// allow our notification to cancel it.
	query_ctx, query_cancel := context.WithCancel(ctx)
	defer query_cancel()

	// Run this query as the specified username
	acl_manager := acl_managers.NewServerACLManager(config_obj, user_name)

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}
	global_repo, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	tmpl, err := reporting.NewGuiTemplateEngine(
		config_obj, query_ctx, nil, acl_manager, global_repo,
		notebook_path_manager.Cell(in.CellId, in.Version),
		"Server.Internal.ArtifactDescription")
	if err != nil {
		logger.Debug("NotebookWorker: While evaluating template: %v", err)

		return nil, err
	}
	defer tmpl.Close()

	tmpl.SetEnv("NotebookId", in.NotebookId)
	tmpl.SetEnv("NotebookCellId", in.CellId)

	// Throttle the notebook accordingly.
	tmpl.Scope.SetContext(constants.SCOPE_QUERY_NAME,
		fmt.Sprintf("Notebook %v", in.NotebookId))
	t, closer := throttler.NewThrottler(ctx, tmpl.Scope, config_obj, 0, 0, 0)
	tmpl.Scope.SetThrottler(t)
	err = tmpl.Scope.AddDestructor(closer)
	if err != nil {
		return nil, err
	}

	// Register a progress reporter so we can monitor how the
	// template rendering is going.
	tmpl.Progress = &progressReporter{
		config_obj:    config_obj,
		notebook_cell: notebook_cell,
		notebook_id:   in.NotebookId,
		version:       in.Version,
		start:         utils.GetTime().Now(),
		store:         store,
		tmpl:          tmpl,
	}

	// Add the notebook environment into the cell template.
	for _, env := range notebook_metadata.Env {
		tmpl.SetEnv(env.Key, env.Value)
	}

	// Also apply the cell env
	for _, env := range in.Env {
		tmpl.SetEnv(env.Key, env.Value)
	}

	// Initialize the cell from the notebook_metadata
	for _, req := range notebook_metadata.Requests {
		// First populate all the Env from requests.
		for _, env := range req.Env {
			tmpl.SetEnv(env.Key, env.Value)
		}

		// Next execute all the queries - discard the output though.
		for _, query := range req.Query {
			tmpl.Query(query.VQL)
		}
	}

	input := in.Input
	cell_type := in.Type

	// Update the content asynchronously
	start_time := utils.GetTime().Now()

	notifier, err := services.GetNotifier(config_obj)
	if err != nil {
		return nil, err
	}

	// The notification is removed either inline or in the background.
	cancel_notify, remove_notification := notifier.ListenForNotification(
		in.CellId + in.Version)

	// Watcher thread: Wait for cancellation from the GUI or a 10 min timeout.
	go func() {
		defer query_cancel()
		defer remove_notification()
		defer func() {
			utils.GetTime().Sleep(time.Second)
			runtime.GC()
		}()

		default_notebook_expiry := config_obj.Defaults.NotebookCellTimeoutMin
		if default_notebook_expiry == 0 {
			default_notebook_expiry = 10
		}

		select {
		case <-query_ctx.Done():
			return

		// Active cancellation from the GUI.
		case <-cancel_notify:
			tmpl.Scope.Log("ERROR:Cancelled after %v !",
				utils.GetTime().Now().Sub(start_time))

			// Set a timeout.
		case <-time.After(utils.Jitter(time.Duration(default_notebook_expiry) * time.Minute)):
			tmpl.Scope.Log("ERROR:Query timed out after %v !",
				utils.GetTime().Now().Sub(start_time))
		}
	}()

	resp, err := self.updateCellContents(query_ctx,
		config_obj, store, tmpl,
		in.CurrentlyEditing, in.NotebookId,
		in.CellId, in.Version, in.AvailableVersions,
		cell_type, in.Env, query_cancel, input, in.Input)
	if err != nil {
		logger.Error("Rendering error: %v", err)
	}

	return &NotebookResponse{
		NotebookCell: resp,
	}, err
}

func (self *NotebookWorker) updateCellContents(
	query_ctx context.Context,
	config_obj *config_proto.Config,
	store NotebookStore,
	tmpl *reporting.GuiTemplateEngine,
	currently_editing bool,
	notebook_id, cell_id, version string,
	available_versions []string,
	cell_type string,
	env []*api_proto.Env,
	query_cancel func(),
	input, original_input string) (res *api_proto.NotebookCell, err error) {

	// Start a nanny to watch this calculation
	go self.startNanny(query_ctx, config_obj, tmpl.Scope, store, query_cancel,
		notebook_id, cell_id, version)

	output := ""

	cell_type = strings.ToLower(cell_type)

	// Create a new cell to set the result in.
	make_cell := func(output string) *api_proto.NotebookCell {
		encoded_data, err := json.Marshal(tmpl.Data)
		if err != nil {
			tmpl.Scope.Log("Error: %v", err)
		}

		return &api_proto.NotebookCell{
			Input:            original_input,
			Output:           output,
			Data:             string(encoded_data),
			Messages:         tmpl.Messages(),
			MoreMessages:     tmpl.MoreMessages(),
			CellId:           cell_id,
			Type:             cell_type,
			Env:              env,
			Timestamp:        utils.GetTime().Now().UnixNano(),
			CurrentlyEditing: currently_editing,
			Duration:         int64(time.Since(tmpl.Start).Seconds()),

			// Version management
			CurrentVersion:    version,
			AvailableVersions: available_versions,
		}
	}

	// If an error occurs it is important to ensure the cell is
	// still written with an error message.
	make_error_cell := func(
		output string, err error) (*api_proto.NotebookCell, error) {
		tmpl.Scope.Log("ERROR: %v", err)
		error_cell := make_cell(output)
		error_cell.Calculating = false
		error_cell.Error = err.Error()

		err1 := store.SetNotebookCell(notebook_id, error_cell)
		if err1 != nil {
			return nil, err1
		}

		return error_cell, fmt.Errorf("%w: While rendering notebook cell: %v",
			utils.InlineError, err)
	}

	// Do not let exceptions take down the server.
	defer func() {
		r := recover()
		if r != nil {
			res, err = make_error_cell("", fmt.Errorf(
				"PANIC: %v: %v", r, string(debug.Stack())))
		}
	}()

	// Write a place holder immediately while we calculate the rest.
	notebook_cell := make_cell(output)
	notebook_cell.Calculating = true
	err = store.SetNotebookCell(notebook_id, notebook_cell)
	if err != nil {
		return nil, err
	}

	waitForMemoryLimit(query_ctx, tmpl.Scope, config_obj)

	switch cell_type {

	case "vql_suggestion", "none":
		// noop - these cells will be created by the user on demand.

	case "markdown", "md":
		// A Markdown cell just feeds directly into the
		// template.
		output, err = tmpl.Execute(&artifacts_proto.Report{Template: input})
		if err != nil {
			return make_error_cell(output, err)
		}

	case "vql":
		// No query, nothing to do
		if reporting.IsEmptyQuery(input) {
			tmpl.Error("Please specify a query to run")
		} else {
			vqls, err := vfilter.MultiParseWithComments(input)
			if err != nil {
				// Try parsing without comments if comment parser fails
				vqls, err = vfilter.MultiParse(input)
				if err != nil {
					return make_error_cell(output, err)
				}
			}

			no_query := true
			for _, vql := range vqls {
				if vql.Comments != nil {
					// Only extract multiline comments to render template
					// Ignore code comments
					comments := multiLineCommentsToString(vql)
					if comments != "" {
						fragment_output, err := tmpl.Execute(
							&artifacts_proto.Report{Template: comments})
						if err != nil {
							return make_error_cell(output, err)
						}
						output += fragment_output
					}
				}
				if vql.Let != "" || vql.Query != nil || vql.StoredQuery != nil {
					no_query = false
					rows, err := tmpl.RunQuery(vql, nil)

					if err != nil {
						return make_error_cell(output, err)
					}

					// VQL Let won't return rows. Ignore
					if vql.Let == "" {
						output_any, ok := tmpl.Table(rows).(string)
						if ok {
							output += output_any
						}
					}
				}
			}
			// No VQL found, only comments
			if no_query {
				tmpl.Error("Please specify a query to run")
			}
		}

	default:
		return make_error_cell(output, errors.New("Unsupported cell type."))
	}

	tmpl.Close()

	notebook_cell = make_cell(output)
	return notebook_cell, store.SetNotebookCell(notebook_id, notebook_cell)
}

func multiLineCommentsToString(vql *vfilter.VQL) string {
	output := ""

	for _, comment := range vql.Comments {
		if comment.MultiLine != nil {
			output += *comment.MultiLine
		}
	}

	if output != "" {
		return output[2 : len(output)-2]
	} else {
		return output
	}
}

func (self *NotebookWorker) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	name string,
	scheduler services.Scheduler) {

	for {
		err := self.RegisterWorker(ctx, config_obj, name, scheduler)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Info("NotebookWorker: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(utils.Jitter(10 * time.Second)):
		}
	}

}

// Register a worker with the scheduler and process any tasks.
func (self *NotebookWorker) RegisterWorker(
	ctx context.Context,
	config_obj *config_proto.Config,
	name string,
	scheduler services.Scheduler) error {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	var priority int64
	if config_obj.Defaults != nil {
		priority = config_obj.Defaults.NotebookWorkerPriority
	}

	if services.IsMinion(config_obj) {
		// By default minion workers have lower priority if not
		// specified to keep the default running notebook processing
		// on the master.
		priority = -10
		if config_obj.Minion != nil &&
			config_obj.Minion.NotebookWorkerPriority != 0 {
			priority = config_obj.Minion.NotebookWorkerPriority
		}
	}

	queue := "Notebook"
	job_chan, err := scheduler.RegisterWorker(ctx, queue, name, int(priority))
	if err != nil {
		return err
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("NotebookWorker: <green> Connecting to master scheduler with worker %v</>",
		name)

	for {
		select {
		case <-ctx.Done():
			return nil

		case job, ok := <-job_chan:
			if !ok {
				if job.Done != nil {
					job.Done("", errors.New("Cancellation"))
				}
				return nil
			}

			request := &NotebookRequest{}
			err := json.Unmarshal([]byte(job.Job), request)
			if err != nil || request.NotebookCellRequest == nil {
				logger.Error("NotebookManager: Invalid job request in worker: %v: %v",
					err, job.Job)
				job.Done("", err)
				continue
			}

			// run the query in the correct ORG. We assume ACL
			// checks occur in the GUI so we can only receive
			// valid requrests here.
			org_config_obj, err := org_manager.GetOrgConfig(job.OrgId)
			if err != nil {
				job.Done("", err)
				continue
			}

			// Run this on the notebook manager that belongs to the org.
			notebook_manager_if, err := services.GetNotebookManager(org_config_obj)
			if err != nil {
				job.Done("", err)
				continue
			}

			notebook_manager, ok := notebook_manager_if.(*NotebookManager)
			if !ok {
				job.Done("", fmt.Errorf("Unsupported notebook manager %T",
					notebook_manager_if))
				continue
			}

			// Call our processor on the correct org.
			resp, err := self.ProcessUpdateRequest(ctx, org_config_obj, request,
				notebook_manager.Store)
			serialized, _ := json.Marshal(resp)
			if resp == nil {
				serialized = nil
			}
			job.Done(string(serialized), err)
		}
	}
}

type WorkerPool struct {
	workers []*NotebookWorker
}

func NewWorkerPool(
	ctx context.Context,
	config_obj *config_proto.Config,
	number int64) (*WorkerPool, error) {
	result := &WorkerPool{}

	scheduler, err := services.GetSchedulerService(config_obj)
	if err != nil {
		return nil, err
	}

	for i := int64(0); i < number; i++ {
		worker := &NotebookWorker{}
		name := fmt.Sprintf("local_notebook_%d", i)
		if !services.IsMaster(config_obj) {
			name = fmt.Sprintf("%v_notebook_%d",
				services.GetNodeName(config_obj.Frontend), i)
		}

		go worker.Start(ctx, config_obj, name, scheduler)
		result.workers = append(result.workers, worker)
	}

	return result, nil
}

func (self *NotebookManager) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	// Only start this once for all orgs. Otherwise we would have as
	// many workers as orgs. Workers will switch into the correct org
	// for processing
	if !utils.IsRootOrg(config_obj.OrgId) {
		return nil
	}

	// Start a few local workers for notebooks. This also limits
	// concurrency on notebook processing.
	local_workers := int64(1)
	if config_obj.Defaults.NotebookNumberOfLocalWorkers != 0 {
		local_workers = config_obj.Defaults.NotebookNumberOfLocalWorkers
	}
	if services.IsMinion(config_obj) && config_obj.Minion != nil &&
		config_obj.Minion.NotebookNumberOfLocalWorkers > 0 {
		local_workers = config_obj.Minion.NotebookNumberOfLocalWorkers
	}

	_, err := NewWorkerPool(ctx, config_obj, local_workers)
	return err
}

func (self *NotebookWorker) startNanny(
	ctx context.Context, config_obj *config_proto.Config,
	scope vfilter.Scope,
	store NotebookStore, query_cancel func(),
	notebook_id, cell_id, version string) {

	// Reduce memory use now so the next measure of memory use is more
	// reflective of our current workload.
	runtime.GC()

	// Running in a goroutine it's ok to block.
	for {

		// Check for high memory use.
		if config_obj.Defaults != nil &&
			config_obj.Defaults.NotebookMemoryHighWaterMark > 0 {

			high_memory_level := config_obj.Defaults.NotebookMemoryHighWaterMark

			var m runtime.MemStats
			// NOTE: We assume this is reasonable fast as we check it
			// frequently.
			runtime.ReadMemStats(&m)

			// If we exceed memory we cancel this query.
			if high_memory_level < m.Alloc {
				scope.Log("ERROR:Insufficient resourcs: Query cancelled. Memory used %v, Limit %v",
					m.Alloc, high_memory_level)
				query_cancel()
			}
		}

		select {
		case <-ctx.Done():
			return

			// Wait for a bit then check if the query is cancelled.
		case <-time.After(utils.Jitter(time.Second)):
		}

		// Check the cell for cancellation or errors
		notebook_cell, err := store.GetNotebookCell(notebook_id, cell_id, version)
		if err != nil || notebook_cell.CellId != cell_id {
			continue
		}

		// Cancel the query - this cell is no longer running
		if !notebook_cell.Calculating {
			// Notify the calculator immediately
			notifier, err := services.GetNotifier(config_obj)
			if err != nil {
				return
			}
			scope.Log("ERROR:NotebookManager: Detected cell %v is cancelled. Stopping.", cell_id)
			notifier.NotifyDirectListener(cell_id + version)

			return
		}
	}
}

func waitForMemoryLimit(
	ctx context.Context, scope types.Scope, config_obj *config_proto.Config) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	// Wait until memory is below the low water mark.
	if config_obj.Defaults != nil &&
		config_obj.Defaults.NotebookMemoryLowWaterMark > 0 {

		low_memory_level := config_obj.Defaults.NotebookMemoryLowWaterMark

		for {
			// Reduce memory use now so the next measure of memory use
			// is more reflective of our current workload.
			runtime.GC()

			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			// Spin here until there is enough memory
			if low_memory_level > m.Alloc {
				break
			}

			functions.DeduplicatedLog(ctx, scope,
				"INFO:Waiting for memory use to allow starting the query.")

			logger.Debug("Waiting for memory use to allow starting the query (current memory %v, low water mark %v)",
				m.Alloc, low_memory_level)

			select {
			case <-ctx.Done():
				scope.Log("ERROR:Unable to start query before timeout - insufficient resourcs.")
				return
			case <-time.After(utils.Jitter(time.Second)):
			}
		}
	}
}
