package launcher

import (
	"context"
	"fmt"
	"os"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *Launcher) DeleteFlow(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, flow_id string,
	really_do_it bool) ([]*services.DeleteFlowResponse, error) {

	collection_details, err := self.GetFlowDetails(config_obj, client_id, flow_id)
	if err != nil {
		return nil, err
	}

	collection_context := collection_details.Context
	if collection_context == nil {
		return nil, nil
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)

	upload_metadata_path := flow_path_manager.UploadMetadata()
	r := &reporter{
		really_do_it: really_do_it,
		ctx:          ctx,
		config_obj:   config_obj,
		seen:         make(map[string]bool),
	}
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, flow_path_manager.UploadMetadata())
	if err == nil {
		for row := range reader.Rows(ctx) {
			upload, pres := row.GetString("vfs_path")
			if pres {
				// Each row is the full filestore path of the upload.
				pathspec := path_specs.NewUnsafeFilestorePath(
					utils.SplitComponents(upload)...).
					SetType(api.PATH_TYPE_FILESTORE_ANY)

				r.emit_fs("Upload", pathspec)
			}
		}
	}

	// Order results to facilitate deletion - container deletion
	// happens after we read its contents.
	r.emit_fs("UploadMetadata", upload_metadata_path)
	r.emit_fs("UploadMetadataIndex", upload_metadata_path.
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))

	// Remove all result sets from artifacts.
	for _, artifact_name := range collection_context.ArtifactsWithResults {
		path_manager, err := artifact_paths.NewArtifactPathManager(
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

	}

	r.emit_fs("Log", flow_path_manager.Log())
	r.emit_fs("LogIndex", flow_path_manager.Log().
		SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
	r.emit_ds("CollectionContext", flow_path_manager.Path())
	r.emit_ds("Task", flow_path_manager.Task())

	// Walk the flow's datastore and filestore
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	r.emit_ds("Notebook", flow_path_manager.Notebook().Path())
	datastore.Walk(config_obj, db, flow_path_manager.Notebook().DSDirectory(),
		func(path api.DSPathSpec) error {
			r.emit_ds("NotebookData", path)
			return nil
		})

	api.Walk(file_store_factory,
		flow_path_manager.Notebook().Directory(),
		func(path api.FSPathSpec, info os.FileInfo) error {
			r.emit_fs("NotebookItem", path)
			return nil
		})

	return r.responses, nil
}

type reporter struct {
	ctx          context.Context
	responses    []*services.DeleteFlowResponse
	seen         map[string]bool
	config_obj   *config_proto.Config
	really_do_it bool
}

func (self *reporter) emit_ds(
	item_type string, target api.DSPathSpec) {
	client_path := target.String()
	var error_message string

	if self.seen[client_path] {
		return
	}
	self.seen[client_path] = true

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
		Type:  item_type,
		Data:  ordereddict.NewDict().Set("VFSPath", client_path),
		Error: error_message,
	})
}

func (self *reporter) emit_fs(
	item_type string, target api.FSPathSpec) {
	client_path := target.String()
	var error_message string

	if self.seen[client_path] {
		return
	}
	self.seen[client_path] = true

	if self.really_do_it {
		file_store_factory := file_store.GetFileStore(self.config_obj)
		err := file_store_factory.Delete(target)
		if err != nil {
			error_message = fmt.Sprintf(
				"Error deleting %v: %v", client_path, err)
		}
	}

	self.responses = append(self.responses, &services.DeleteFlowResponse{
		Type:  item_type,
		Data:  ordereddict.NewDict().Set("VFSPath", client_path),
		Error: error_message,
	})
}
