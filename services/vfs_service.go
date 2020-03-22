// This is the VFSService.
//
// This service watches for flow completions of
// System.VFS.ListDirectory and System.VFS.DownloadFile. When these
// artifacts are detected, the service will update the VFS tree with
// the new directory data or a reference to the downloaded files.
package services

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/parsers"
	"www.velocidex.com/golang/vfilter"
)

type VFSService struct {
	mu sync.Mutex

	config_obj *config_proto.Config
	logger     *logging.LogContext
}

func (self *VFSService) Start(
	ctx context.Context,
	wg *sync.WaitGroup) error {
	self.logger.Info("Starting VFS writing service.")

	err := watchForFlowCompletion(
		ctx, wg, self.config_obj, "System.VFS.ListDirectory",
		self.ProcessListDirectory)
	if err != nil {
		return err
	}

	err = watchForFlowCompletion(
		ctx, wg, self.config_obj, "System.VFS.DownloadFile",
		self.ProcessDownloadFile)
	if err != nil {
		return err
	}

	return nil
}

func (self *VFSService) ProcessDownloadFile(
	ctx context.Context, scope *vfilter.Scope, row vfilter.Row) {

	client_id := vql_subsystem.GetStringFromRow(scope, row, "ClientId")
	flow_id := vql_subsystem.GetStringFromRow(scope, row, "FlowId")
	ts := vql_subsystem.GetIntFromRow(scope, row, "_ts")

	// We need to run a query referring to the row.
	sub_scope := scope.Copy()
	sub_scope.AppendVars(row)

	vql, err := vfilter.Parse(
		"select Path, Accessor FROM source(" +
			"flow_id=FlowId, " +
			"artifact='System.VFS.DownloadFile', " +
			"client_id=ClientId)")
	if err != nil {
		panic(err)
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		panic(err)
	}

	for row := range vql.Eval(ctx, sub_scope) {
		Accessor := vql_subsystem.GetStringFromRow(scope, row, "Accessor")
		Path := vql_subsystem.GetStringFromRow(scope, row, "Path")

		// Figure out where the file was uploaded to.
		vfs_path := artifacts.GetUploadsFile(client_id, flow_id, Accessor, Path)

		// Check to make sure the file actually exists.
		file_store_factory := file_store.GetFileStore(self.config_obj)
		_, err := file_store_factory.StatFile(vfs_path)
		if err != nil {
			self.logger.Error("Unable to save flow %v", err)
			return
		}

		// We store a place holder in the VFS pointing at the
		// read vfs_path of the download.
		err = db.SetSubject(self.config_obj,
			artifacts.GetVFSDownloadInfoPath(client_id, Accessor, Path),
			&flows_proto.VFSDownloadInfo{
				VfsPath: vfs_path,
				Mtime:   uint64(ts) * 1000000,
				Size:    vql_subsystem.GetIntFromRow(scope, row, "Size"),
			})
		if err != nil {
			self.logger.Error("Unable to save flow %v", err)
		}
	}
}

func (self *VFSService) ProcessListDirectory(
	ctx context.Context, scope *vfilter.Scope, row vfilter.Row) {

	client_id := vql_subsystem.GetStringFromRow(scope, row, "ClientId")
	flow_id := vql_subsystem.GetStringFromRow(scope, row, "FlowId")
	ts := vql_subsystem.GetIntFromRow(scope, row, "_ts")

	// We need to run a query referring to the row.
	sub_scope := scope.Copy()
	sub_scope.AppendVars(row)

	vql, err := vfilter.Parse("select * FROM " +
		"source(artifact='System.VFS.ListDirectory', " +
		"flow_id=FlowId, client_id=ClientId)")
	if err != nil {
		panic(err)
	}

	var rows []vfilter.Row
	var current_vfs_components []string = nil

	for row := range vql.Eval(ctx, sub_scope) {
		full_path := vql_subsystem.GetStringFromRow(scope, row, "_FullPath")
		accessor := vql_subsystem.GetStringFromRow(scope, row, "_Accessor")
		name := vql_subsystem.GetStringFromRow(scope, row, "Name")

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
				err := self.flush_state(
					sub_scope, ts, client_id,
					flow_id,
					current_vfs_components, rows)
				if err != nil {
					return
				}
				rows = nil
			}
			current_vfs_components = dir_components
		}
		rows = append(rows, row)
	}

	err = self.flush_state(sub_scope, ts, client_id, flow_id,
		current_vfs_components, rows)
	if err != nil {
		self.logger.Error("Unable to save directory: %v", err)
		return
	}
}

// Flush the current state into the database and clear it for the next directory.
func (self *VFSService) flush_state(scope *vfilter.Scope,
	timestamp uint64, client_id, flow_id string,
	vfs_components []string, rows []vfilter.Row) error {
	if len(rows) == 0 {
		return nil
	}

	serialized, err := json.Marshal(rows)
	if err != nil {
		return errors.WithStack(err)
	}

	urn := utils.JoinComponents(append([]string{
		"clients", client_id, "vfs"}, vfs_components...), "/")

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}
	return db.SetSubject(self.config_obj,
		urn, &flows_proto.VFSListResponse{
			Columns:   scope.GetMembers(rows[0]),
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
		device, subpath, err := parsers.GetDeviceAndSubpath(client_path)
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

func startVFSService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {
	vfs_service := &VFSService{
		config_obj: config_obj,
		logger:     logging.GetLogger(config_obj, &logging.FrontendComponent),
	}

	return vfs_service.Start(ctx, wg)
}
