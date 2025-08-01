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

	upload_metadata_path := flow_path_manager.UploadMetadata()

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
			components, pres := row.GetStrings("_Components")
			if pres {
				pathspec := path_specs.NewUnsafeFilestorePath(
					components...).SetType(api.PATH_TYPE_FILESTORE_ANY)
				r.emit_fs("Upload", pathspec)
				r.emit_fs("UploadIdx", pathspec.
					SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX))
				r.emit_fs("UploadChunk", pathspec.
					SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))
				continue
			}

			upload, pres := row.GetString("vfs_path")
			if pres {
				// Each row is the full filestore path of the upload.
				pathspec := path_specs.NewUnsafeFilestorePath(
					utils.SplitComponents(upload)...).
					SetType(api.PATH_TYPE_FILESTORE_ANY)

				r.emit_fs("Upload", pathspec)
				r.emit_fs("UploadIdx", pathspec.
					SetType(api.PATH_TYPE_FILESTORE_SPARSE_IDX))
				r.emit_fs("UploadChunk", pathspec.
					SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))
			}
		}
		reader.Close()
	}

	// Order results to facilitate deletion - container deletion
	// happens after we read its contents.
	r.emit_fs("UploadMetadata", upload_metadata_path)
	r.emit_fs("UploadMetadataIndex", upload_metadata_path.
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))

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
		r.emit_fs("Result", result_path)
		r.emit_fs("ResultIndex",
			result_path.SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
		r.emit_fs("ResultChunkIndex",
			result_path.SetType(api.PATH_TYPE_FILESTORE_CHUNK_INDEX))

	}

	r.emit_fs("Log", flow_path_manager.Log())
	r.emit_fs("LogIndex", flow_path_manager.Log().
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
	r.emit_ds("CollectionContext", flow_path_manager.Path())
	r.emit_ds("Task", flow_path_manager.Task())
	r.emit_ds("Stats", flow_path_manager.Stats())

	// Walk the flow's datastore and filestore
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	r.emit_ds("Notebook", flow_path_manager.Notebook().Path())
	_ = datastore.Walk(config_obj, db, flow_path_manager.Notebook().DSDirectory(),
		datastore.WalkWithoutDirectories,
		func(path api.DSPathSpec) error {
			r.emit_ds("NotebookData", path)
			return nil
		})

	// Clean the empty directories
	_ = datastore.Walk(config_obj, db, flow_path_manager.Notebook().DSDirectory(),
		datastore.WalkWithDirectories,
		func(path api.DSPathSpec) error {
			_ = db.DeleteSubject(config_obj, path)
			return nil
		})

	_ = api.Walk(file_store_factory,
		flow_path_manager.Notebook().Directory(),
		func(path api.FSPathSpec, info os.FileInfo) error {
			r.emit_fs("NotebookItem", path)
			return nil
		})

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
	r.pool.StopAndWait()

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

func (self *reporter) emit_ds(
	item_type string, target api.DSPathSpec) {

	client_path := target.String()
	var error_message string

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.seen[client_path] {
		return
	}
	self.seen[client_path] = true

	self.id++
	id := self.id

	self.pool.Submit(func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		if self.really_do_it {
			db, err := datastore.GetDB(self.config_obj)
			if err == nil {
				err = db.DeleteSubject(self.config_obj, target)
				if err != nil {
					error_message = fmt.Sprintf(
						"Error deleting %v: %v", client_path, err)
				}
			}
		}

		self.responses = append(self.responses, &services.DeleteFlowResponse{
			Id:    id,
			Type:  item_type,
			Data:  ordereddict.NewDict().Set("VFSPath", client_path),
			Error: error_message,
		})
	})

}

func (self *reporter) emit_fs(
	item_type string, target api.FSPathSpec) {
	client_path := target.String()
	var error_message string

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.seen[client_path] {
		return
	}
	self.seen[client_path] = true

	self.id++
	id := self.id

	self.pool.Submit(func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		if self.really_do_it {
			file_store_factory := file_store.GetFileStore(self.config_obj)
			err := file_store_factory.Delete(target)
			if err != nil {
				error_message = fmt.Sprintf(
					"Error deleting %v: %v", client_path, err)
			}
		}

		self.responses = append(self.responses, &services.DeleteFlowResponse{
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
