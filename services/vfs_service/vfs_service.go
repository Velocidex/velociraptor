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
	"github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
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
	logger.Info("<green>Starting</> VFS writing service.")

	err := watchForFlowCompletion(
		ctx, wg, config_obj, "System.VFS.ListDirectory",
		self.ProcessListDirectory)
	if err != nil {
		return err
	}

	err = watchForFlowCompletion(
		ctx, wg, config_obj, "System.VFS.DownloadFile",
		self.ProcessDownloadFile)
	if err != nil {
		return err
	}

	return nil
}

func (self *VFSService) ProcessDownloadFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope, row *ordereddict.Dict) {

	defer utils.CheckForPanic("ProcessDownloadFile")

	client_id, _ := row.GetString("ClientId")
	flow_id, _ := row.GetString("FlowId")
	ts, _ := row.GetInt64("_ts")

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("VFSService: Processing System.VFS.DownloadFile from %v", client_id)

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)

	path_manager := artifacts.NewArtifactPathManager(config_obj,
		client_id, flow_id, "System.VFS.DownloadFile")
	row_chan, err := file_store.GetTimeRange(
		ctx, config_obj, path_manager, 0, 0)
	if err != nil {
		logger.Error("Unable to read artifact: %v", err)
		return
	}

	for row := range row_chan {
		Accessor, _ := row.GetString("Accessor")
		Path, _ := row.GetString("Path")
		MD5, _ := row.GetString("Md5")
		SHA256, _ := row.GetString("Sha256")

		// Figure out where the file was uploaded to.
		vfs_path_manager := flow_path_manager.GetUploadsFile(Accessor, Path)

		// Check to make sure the file actually exists.
		file_store_factory := file_store.GetFileStore(config_obj)
		_, err := file_store_factory.StatFile(vfs_path_manager.Path())
		if err != nil {
			logger.Error(
				"Unable to save flow %v: %v",
				vfs_path_manager.Path(), err)
			continue
		}

		_, err = file_store_factory.StatFile(vfs_path_manager.IndexPath())

		// We store a place holder in the VFS pointing at the
		// read vfs_path of the download.
		db, _ := datastore.GetDB(config_obj)
		err = db.SetSubject(config_obj,
			flow_path_manager.GetVFSDownloadInfoPath(Accessor, Path).Path(),
			&flows_proto.VFSDownloadInfo{
				VfsPath: vfs_path_manager.Path(),
				Mtime:   uint64(ts) * 1000000,
				Sparse:  err == nil, // If index file exists we have an index.
				Size:    vql_subsystem.GetIntFromRow(scope, row, "Size"),
				MD5:     MD5,
				SHA256:  SHA256,
			})
		if err != nil {
			logger.Error(
				"Unable to save flow %v: %v",
				vfs_path_manager.Path(), err)
		}
	}
}

func (self *VFSService) ProcessListDirectory(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope, row *ordereddict.Dict) {

	client_id, _ := row.GetString("ClientId")
	flow_id, _ := row.GetString("FlowId")
	ts, _ := row.GetInt64("_ts")

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("VFSService: Processing System.VFS.ListDirectory from %v", client_id)

	path_manager := artifacts.NewArtifactPathManager(config_obj,
		client_id, flow_id, "System.VFS.ListDirectory")

	row_chan, err := file_store.GetTimeRange(
		ctx, config_obj, path_manager, 0, 0)
	if err != nil {
		logger.Error("Unable to read artifact: %v", err)
		return
	}

	var rows []*ordereddict.Dict
	var current_vfs_components []string = nil

	for row := range row_chan {
		full_path, _ := row.GetString("_FullPath")
		accessor, _ := row.GetString("_Accessor")
		name, _ := row.GetString("Name")

		if name == "." || name == ".." {
			continue
		}

		vfs_components := getVfsComponents(full_path, accessor)

		if vfs_components == nil {
			continue
		}

		dir_components := vfs_components[:len(vfs_components)-1]

		// This row does not belong in the current
		// collection - flush the collection and start
		// a new one.
		if !utils.StringSliceEq(dir_components, current_vfs_components) ||

			// Do not let our memory footprint
			// grow without bounds.
			len(rows) > 100000 {

			// current_vfs_components == nil represents
			// the first collection before the first row
			// is processed.
			if current_vfs_components != nil {
				err := self.flush_state(config_obj, uint64(ts), client_id,
					flow_id, current_vfs_components, rows)
				if err != nil {
					return
				}
				rows = nil
			}
			current_vfs_components = dir_components
		}
		rows = append(rows, row)
	}

	err = self.flush_state(config_obj, uint64(ts), client_id, flow_id,
		current_vfs_components, rows)
	if err != nil {
		logger.Error("Unable to save directory: %v", err)
		return
	}
}

// Flush the current state into the database and clear it for the next directory.
func (self *VFSService) flush_state(
	config_obj *config_proto.Config, timestamp uint64, client_id, flow_id string,
	vfs_components []string, rows []*ordereddict.Dict) error {
	if len(rows) == 0 {
		return nil
	}

	serialized, err := json.Marshal(rows)
	if err != nil {
		return errors.WithStack(err)
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	client_path_manager := paths.NewClientPathManager(client_id)
	return db.SetSubject(config_obj,
		client_path_manager.VFSPath(vfs_components),
		&flows_proto.VFSListResponse{
			Columns:   rows[0].Keys(),
			Timestamp: timestamp,
			Response:  string(serialized),
			TotalRows: uint64(len(rows)),
			ClientId:  client_id,
			FlowId:    flow_id,
		})
}

// The inverse of GetClientPath()
func getVfsComponents(client_path string, accessor string) []string {
	switch accessor {

	case "reg", "registry":
		return append([]string{"registry"}, utils.SplitComponents(client_path)...)

	case "ntfs":
		device, subpath, err := paths.GetDeviceAndSubpath(client_path)
		if err == nil {
			if subpath == "" || subpath == "." {
				return []string{"ntfs", device}
			}
			return append([]string{"ntfs", device},
				utils.SplitPlainComponents(subpath)...)
		}
	}

	return append([]string{"file"},
		utils.SplitPlainComponents(client_path)...)
}

func StartVFSService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	vfs_service := &VFSService{}

	return vfs_service.Start(ctx, config_obj, wg)
}
