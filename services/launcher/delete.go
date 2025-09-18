package launcher

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alitto/pond/v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func (self *FlowStorageManager) DeleteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, flow_id string, principal string,
	options services.DeleteFlowOptions) (
	[]*services.DeleteFlowResponse, error) {

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return nil, err
	}

	collection_details, err := launcher.GetFlowDetails(
		ctx, config_obj, services.GetFlowOptions{},
		client_id, flow_id)
	if err != nil {
		return nil, err
	}

	collection_context := collection_details.Context
	if collection_context == nil {
		return nil, nil
	}

	if options.ReallyDoIt && principal != "" {
		err := services.LogAudit(ctx,
			config_obj, principal, "delete_flow",
			ordereddict.NewDict().
				Set("client_id", client_id).
				Set("flow_id", flow_id).
				Set("flow", collection_context))
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("<red>FlowStorageManager delete_flow</> %v %v %v",
				principal, client_id, flow_id)
		}
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	flow_base_path := flow_path_manager.Path().Components()

	r := &reporter{
		really_do_it: options.ReallyDoIt,
		ctx:          ctx,
		config_obj:   config_obj,
		seen:         make(map[string]bool),
		pool:         pond.NewPool(100),
	}
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, flow_path_manager.UploadMetadata())
	if err == nil {
		for row := range reader.Rows(ctx) {
			// Some uploads list components relative to the client.
			components, pres := row.GetStrings("_Components")
			if pres && len(components) > 0 {

				// Make sure the uploads exist within this flow.
				if !utils.SlicePrefixMatch(components, flow_base_path) {
					continue
				}

				pathspec := path_specs.NewUnsafeFilestorePath(
					components...).SetType(api.PATH_TYPE_FILESTORE_ANY)
				r.emit_bulk_file("Upload", pathspec)
				continue
			}
		}
		reader.Close()
	}

	// Order results to facilitate deletion - container deletion
	// happens after we read its contents.
	r.emit_result_set("UploadMetadata", flow_path_manager.UploadMetadata())
	r.emit_result_set("UploadTransactions",
		flow_path_manager.UploadTransactions())

	// Remove all result sets from artifacts.
	for _, artifact_name := range collection_context.ArtifactsWithResults {
		path_manager, err := artifact_paths.NewArtifactPathManager(ctx,
			config_obj, client_id, flow_id, artifact_name)
		if err != nil {
			continue
		}

		result_path, err := path_manager.GetPathForWriting()
		if err != nil {
			continue
		}
		r.emit_result_set("Result", result_path)
	}

	r.emit_result_set("Log", flow_path_manager.Log())

	r.emit_ds("CollectionContext", flow_path_manager.Path())
	r.emit_ds("Task", flow_path_manager.Task())
	r.emit_ds("Stats", flow_path_manager.Stats())

	// Walk the flow's datastore and filestore
	r.emit_notebook("Notebook", flow_path_manager.Notebook())

	if options.ReallyDoIt {
		// User specified the flow must be removed immediately.
		if options.Sync {
			err = self.RemoveClientFlowsFromIndex(
				ctx, config_obj, client_id, map[string]bool{
					flow_id: true,
				})
		} else {
			// Otherwise we just mark the index as pending a rebuild
			// and move on.
			err = self.writeFlowJournal(config_obj, client_id, flow_id)
		}
	}
	r.wait()

	// Wait for all the deletions to finish then delete anything left
	// over that we missed. This should help trap future missed items
	if options.ReallyDoIt {
		r.reset()
		r.emit_walk_fs("Unknown",
			flow_path_manager.Path().AsFilestorePath().
				SetType(api.PATH_TYPE_FILESTORE_ANY))
		r.wait()
	}

	// Sort responses to keep output stable
	sort.Slice(r.responses, func(i, j int) bool {
		return r.responses[i].Id < r.responses[j].Id
	})

	return r.responses, err
}

type reporter struct {
	ctx          context.Context
	responses    []*services.DeleteFlowResponse
	seen         map[string]bool
	config_obj   *config_proto.Config
	really_do_it bool
	mu           sync.Mutex
	id           int
	pool         pond.Pool
}

func (self *reporter) reset() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.pool = pond.NewPool(10)
}

func (self *reporter) wait() {
	self.mu.Lock()
	pool := self.pool
	self.mu.Unlock()

	pool.StopAndWait()
}

func (self *reporter) emit_ds(
	item_type string, target api.DSPathSpec) {
	self.emit(item_type, target.String(), func() error {
		db, err := datastore.GetDB(self.config_obj)
		if err != nil {
			return err
		}
		return db.DeleteSubject(self.config_obj, target)
	})
}

func (self *reporter) emit_result_set(
	item_type string, target api.FSPathSpec) {

	self.emit(item_type, target.String(), func() error {
		file_store_factory := file_store.GetFileStore(self.config_obj)
		return result_sets.DeleteResultSet(file_store_factory, target)
	})
}

func (self *reporter) emit_notebook(
	item_type string, notebook_path_manager *paths.NotebookPathManager) {

	id := self.get_id()

	self.pool.Submit(func() {
		notebook_manager, err := services.GetNotebookManager(self.config_obj)
		if err != nil {
			return
		}
		output_chan := make(chan vfilter.Row)

		go func() {
			defer close(output_chan)

			err = notebook_manager.DeleteNotebook(
				self.ctx, notebook_path_manager.NotebookId(), output_chan,
				self.really_do_it)
			if err != nil {
				self.add_response(&services.DeleteFlowResponse{
					Type: "Notebook",
					Id:   id,
					Data: ordereddict.NewDict().Set("VFSPath",
						notebook_path_manager.Path()),
					Error: err.Error(),
				})

			}
		}()

		for row := range output_chan {
			row_dict, ok := row.(*ordereddict.Dict)
			if !ok {
				continue
			}
			self.add_response(&services.DeleteFlowResponse{
				Id:   self.get_id(),
				Type: "NotebookData",
				Data: row_dict,
			})
		}
	})
}

func (self *reporter) emit_walk_fs(
	item_type string, target api.FSPathSpec) {

	self.pool.Submit(func() {
		file_store_factory := file_store.GetFileStore(self.config_obj)
		_ = api.Walk(file_store_factory, target,
			func(urn api.FSPathSpec, info os.FileInfo) error {
				error_message := ""
				if !self.should_do_it() {
					err := file_store_factory.Delete(urn)
					if err != nil {
						error_message = err.Error()
					}
				}

				self.add_response(&services.DeleteFlowResponse{
					Id:   self.get_id(),
					Type: item_type,
					Data: ordereddict.NewDict().
						Set("VFSPath", urn.String()).
						Set("Size", info.Size()),
					Error: error_message,
				})
				return nil
			})
	})
}

func (self *reporter) emit_bulk_file(
	item_type string, target api.FSPathSpec) {

	self.emit(item_type, target.String(), func() error {
		file_store_factory := file_store.GetFileStore(self.config_obj)
		return file_store.DeleteBulkFile(file_store_factory, target)
	})
}

func (self *reporter) should_do_it() bool {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.really_do_it
}

func (self *reporter) deduplicate(client_path string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.seen[client_path] {
		return true
	}
	self.seen[client_path] = true
	return false
}

func (self *reporter) get_id() int {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.id++
	return self.id
}
func (self *reporter) add_response(response *services.DeleteFlowResponse) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.responses = append(self.responses, response)
}

func (self *reporter) emit(
	item_type string, client_path string,
	deleter func() error) {

	if self.deduplicate(client_path) {
		return
	}

	id := self.get_id()

	self.pool.Submit(func() {
		var error_message string

		if self.should_do_it() {
			err := deleter()
			if err != nil {
				error_message = fmt.Sprintf(
					"Error deleting %v: %v", client_path, err)
			}
		}

		self.add_response(&services.DeleteFlowResponse{
			Id:    id,
			Type:  item_type,
			Data:  ordereddict.NewDict().Set("VFSPath", client_path),
			Error: error_message,
		})
	})
}

/*
For now we do not bisect the event log files - we just remove the
entire file if the time stamp requested is in it. Since files are
split by day this will remove the entire day's worth of data.
*/
func (self *Launcher) DeleteEvents(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal, artifact, client_id string,
	start_time, end_time time.Time,
	options services.DeleteFlowOptions) (
	[]*services.DeleteFlowResponse, error) {

	path_manager, err := artifacts.NewArtifactPathManager(ctx,
		config_obj, client_id, "", artifact)
	if err != nil {
		return nil, err
	}
	if !path_manager.IsEvent() {
		return nil, fmt.Errorf("Artifact %v is not an event artifact", artifact)
	}

	file_store_factory := file_store.GetFileStore(config_obj)

	result := []*services.DeleteFlowResponse{}
	for _, f := range path_manager.GetAvailableFiles(ctx) {
		if f.EndTime.After(start_time) &&
			f.StartTime.Before(end_time) {
			var error_message string

			if options.ReallyDoIt {
				err := file_store_factory.Delete(f.Path)
				if err != nil {
					error_message = fmt.Sprintf(
						"Error deleting %v: %v",
						f.Path.AsClientPath(), err)
				}

				path_spec := f.Path.SetType(api.PATH_TYPE_FILESTORE_JSON_TIME_INDEX)
				err = file_store_factory.Delete(path_spec)
				if err != nil {
					error_message += fmt.Sprintf(
						"Error deleting %v: %v",
						path_spec.AsClientPath(), err)
				}
			}

			result = append(result, &services.DeleteFlowResponse{
				Type: "EventFile",
				Data: ordereddict.NewDict().Set(
					"VFSPath", f.Path.AsClientPath()),
				Error: error_message,
			})
		}
	}

	log_path_manager, err := artifacts.NewArtifactLogPathManager(ctx,
		config_obj, client_id, "", artifact)
	if err != nil {
		return nil, err
	}
	for _, f := range log_path_manager.GetAvailableFiles(ctx) {
		if f.EndTime.After(start_time) &&
			f.StartTime.Before(end_time) {
			var error_message string

			if options.ReallyDoIt {
				err := file_store_factory.Delete(f.Path)
				if err != nil {
					error_message = fmt.Sprintf(
						"Error deleting %v: %v",
						f.Path.AsClientPath(), err)
				}

				path_spec := f.Path.SetType(api.PATH_TYPE_FILESTORE_JSON_TIME_INDEX)
				err = file_store_factory.Delete(path_spec)
				if err != nil {
					error_message += fmt.Sprintf(
						"Error deleting %v: %v",
						path_spec.AsClientPath(), err)
				}
			}

			result = append(result, &services.DeleteFlowResponse{
				Type: "EventQueryLogFile",
				Data: ordereddict.NewDict().Set(
					"VFSPath", f.Path.AsClientPath()),
				Error: error_message,
			})
		}
	}

	// Log into the audit log
	if options.ReallyDoIt {
		return result, services.LogAudit(ctx, config_obj, principal, "DeleteEvents",
			ordereddict.NewDict().
				Set("artifact", artifact).
				Set("client_id", client_id).
				Set("start_time", start_time).
				Set("end_time", end_time))
	}

	return result, nil
}
