// This is the VFSService.
//
// This service watches for flow completions of
// System.VFS.ListDirectory and System.VFS.DownloadFile. When these
// artifacts are detected, the service will update the VFS tree with
// the new directory data or a reference to the downloaded files.
package vfs_service

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type VFSService struct{}

func (self *VFSService) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> VFS writing service for %v.",
		services.GetOrgName(config_obj))

	// Monitor for legacy ListDirectory flows for clients that do not
	// have specialized vfs_ls() plugins.
	err := watchForFlowCompletion(
		ctx, wg, config_obj, "System.VFS.ListDirectory",
		"VFSService",
		self.ProcessListDirectoryLegacy)
	if err != nil {
		return err
	}

	// Modern clients send stats directly and so do not need special
	// processing - this is much more efficient!
	err = watchForFlowCompletion(
		ctx, wg, config_obj, "System.VFS.ListDirectory/Stats",
		"VFSService",
		self.ProcessListDirectory)
	if err != nil {
		return err
	}

	err = watchForFlowCompletion(
		ctx, wg, config_obj, "System.VFS.DownloadFile",
		"VFSService",
		self.ProcessDownloadFile)
	if err != nil {
		return err
	}

	return nil
}

func (self *VFSService) ProcessDownloadFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope, row *ordereddict.Dict,
	flow *flows_proto.ArtifactCollectorContext) {

	defer utils.CheckForPanic("ProcessDownloadFile")

	client_id, _ := row.GetString("ClientId")
	flow_id, _ := row.GetString("FlowId")
	ts, _ := row.GetInt64("_ts")

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("VFSService: Processing System.VFS.DownloadFile from %v", client_id)

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	client_path_manager := paths.NewClientPathManager(client_id)

	artifact_path_manager, err := artifacts.NewArtifactPathManager(config_obj,
		client_id, flow_id, "System.VFS.DownloadFile")
	if err != nil {
		logger.Error("Unable to read artifact: %v", err)
		return
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, artifact_path_manager.Path())
	if err != nil {
		logger.Error("Unable to read artifact: %v", err)
		return
	}
	defer reader.Close()

	for row := range reader.Rows(ctx) {
		Accessor, _ := row.GetString("Accessor")
		Path, _ := row.GetString("Path")

		MD5, _ := row.GetString("Md5")
		SHA256, _ := row.GetString("Sha256")

		// Figure out where the file was uploaded to.
		uploaded_file_manager := flow_path_manager.GetUploadsFile(
			Accessor, Path)

		// Check to make sure the file actually exists.
		file_store_factory := file_store.GetFileStore(config_obj)
		_, err := file_store_factory.StatFile(uploaded_file_manager.Path())
		if err != nil {
			logger.Error(
				"Unable to save flow %v: %v", Path, err)
			continue
		}

		// Check if the index file exists
		_, err = file_store_factory.StatFile(
			uploaded_file_manager.IndexPath())
		has_index_file := err == nil

		// We store a place holder in the VFS pointing at the
		// read vfs_path of the download.
		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return
		}

		err = db.SetSubject(config_obj,
			client_path_manager.VFSDownloadInfoFromClientPath(
				Accessor, Path),
			&flows_proto.VFSDownloadInfo{
				Components: uploaded_file_manager.
					Path().Components(),
				Mtime:  uint64(ts) * 1000000,
				Sparse: has_index_file, // If index file exists we have an index.
				Size:   vql_subsystem.GetIntFromRow(scope, row, "Size"),
				MD5:    MD5,
				SHA256: SHA256,
			})
		if err != nil {
			logger.Error("Unable to save flow %v: %v", Path, err)
		}
	}
}

// When the VFSListDirectory artifact returns no rows we need to write
// an empty VFS entry but our algorithm relies on the FullPath
// returned by the client in the artifact results. So we need to go
// about it a different way.
func (self *VFSService) handleEmptyListDirectory(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope, row *ordereddict.Dict,
	flow *flows_proto.ArtifactCollectorContext) {

	ts, _ := row.GetInt64("_ts")
	client_id, _ := row.GetString("ClientId")
	flow_id, _ := row.GetString("FlowId")

	vfs_path := findParam("Path", flow)
	accessor := findParam("Accessor", flow)

	// Probably an invalid request.
	if vfs_path == "" || accessor == "" {
		return
	}

	vfs_components := append([]string{accessor},
		paths.ExtractClientPathComponents(vfs_path)...)

	// Write an empty set to the VFS entry.
	err := self.flush_state(config_obj, uint64(ts), client_id, flow_id,
		vfs_components, 0, 0)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("Unable to save directory: %v", err)
		return
	}
}

// Handle older clients that do not have the vfs_ls() plugin.
func (self *VFSService) ProcessListDirectoryLegacy(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope, row *ordereddict.Dict,
	flow *flows_proto.ArtifactCollectorContext) {

	// An empty result set needs special handling.
	if flow.TotalCollectedRows == 0 {
		self.handleEmptyListDirectory(ctx, config_obj, scope, row, flow)
		return
	}

	client_id, _ := row.GetString("ClientId")
	flow_id, _ := row.GetString("FlowId")
	ts, _ := row.GetInt64("_ts")

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("VFSService: Processing System.VFS.ListDirectory from %v", client_id)

	path_manager := artifacts.NewArtifactPathManagerWithMode(
		config_obj, client_id, flow_id, "System.VFS.ListDirectory",
		paths.MODE_CLIENT)

	// Read the results from the flow and build a VFSListResponse
	// for storing in the VFS.
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
	if err != nil {
		logger.Error("Unable to read artifact: %v", err)
		return
	}
	defer reader.Close()

	var current_vfs_components []string = nil

	// In the VFS we store the row range where we can find the files
	// in this directory.
	start_row := 0
	count := 0

	for row := range reader.Rows(ctx) {
		count++

		full_path, _ := row.GetString("_FullPath")
		accessor, _ := row.GetString("_Accessor")
		name, _ := row.GetString("Name")

		if name == "." || name == ".." || name == "" {
			continue
		}

		// Where would this file end up in the VFS?
		file_vfs_path := path_specs.NewUnsafeFilestorePath(accessor).
			AddChild(paths.ExtractClientPathComponents(full_path)...)

		dir_components := file_vfs_path.Dir().Components()
		if len(dir_components) == 0 {
			continue
		}

		// This row does not belong in the current collection - flush
		// the collection and start a new one.
		if !utils.StringSliceEq(dir_components, current_vfs_components) {

			// current_vfs_components == nil represents
			// the first collection before the first row
			// is processed.
			if current_vfs_components != nil {
				err := self.flush_state(
					config_obj, uint64(ts), client_id,
					flow_id, current_vfs_components, start_row, count-1)
				if err != nil {
					return
				}
				start_row = count - 1
			}
			current_vfs_components = dir_components
		}
	}

	err = self.flush_state(config_obj, uint64(ts), client_id, flow_id,
		current_vfs_components, start_row, count)
	if err != nil {
		logger.Error("Unable to save directory: %v", err)
		return
	}

	start_row = count
}

func findParam(name string, flow *flows_proto.ArtifactCollectorContext) string {
	if flow == nil || flow.Request == nil {
		return ""
	}
	for _, spec := range flow.Request.Specs {
		if spec.Parameters == nil {
			continue
		}
		for _, env := range spec.Parameters.Env {
			if env.Key == name {
				return env.Value
			}
		}
	}
	return ""
}

// Flush the current state into the database and clear it for the next directory.
func (self *VFSService) flush_state(
	config_obj *config_proto.Config, timestamp uint64, client_id, flow_id string,
	vfs_components []string, start_idx, end_idx int) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	client_path_manager := paths.NewClientPathManager(client_id)
	return db.SetSubject(config_obj,
		client_path_manager.VFSPath(vfs_components),
		&api_proto.VFSListResponse{
			Timestamp: timestamp,
			TotalRows: uint64(end_idx - start_idx),
			ClientId:  client_id,
			FlowId:    flow_id,
			StartIdx:  uint64(start_idx),
			EndIdx:    uint64(end_idx),
		})
}

// Modern clients do the above work on the client removing load from
// the server. This is much more scalable.
func (self *VFSService) ProcessListDirectory(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope, row *ordereddict.Dict,
	flow *flows_proto.ArtifactCollectorContext) {

	client_id, _ := row.GetString("ClientId")
	flow_id, _ := row.GetString("FlowId")
	ts, _ := row.GetInt64("_ts")

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("VFSService: Processing System.VFS.ListDirectory/Stats from %v", client_id)

	path_manager := artifacts.NewArtifactPathManagerWithMode(
		config_obj, client_id, flow_id, "System.VFS.ListDirectory/Stats",
		paths.MODE_CLIENT)

	// Read the results from the flow and build a VFSListResponse
	// for storing in the VFS.
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
	if err != nil {
		logger.Error("Unable to read artifact: %v", err)
		return
	}
	defer reader.Close()

	json_chan, _ := reader.JSON(ctx)

	for serialized := range json_chan {
		row_obj := &services.VFSListRow{}
		err = json.Unmarshal(serialized, row_obj)
		if err != nil ||
			row_obj.Stats == nil {
			continue
		}

		accessor := row_obj.Accessor
		if accessor == "" {
			accessor = "auto"
		}

		components := append([]string{accessor}, row_obj.Components...)

		// Write missing data from the stats record.
		stats := row_obj.Stats
		stats.Timestamp = uint64(ts)
		stats.ClientId = flow.ClientId
		stats.FlowId = flow.SessionId
		stats.TotalRows = stats.EndIdx - stats.StartIdx
		stats.Artifact = "System.VFS.ListDirectory/Listing"

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return
		}

		// Write the record in the background
		client_path_manager := paths.NewClientPathManager(flow.ClientId)
		_ = db.SetSubjectWithCompletion(config_obj,
			client_path_manager.VFSPath(components), stats, utils.BackgroundWriter)
	}
}

func NewVFSService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (
	services.VFSService, error) {

	vfs_service := &VFSService{}
	return vfs_service, vfs_service.Start(ctx, config_obj, wg)
}
