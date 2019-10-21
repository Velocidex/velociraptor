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
	"path"
	"sync"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/urns"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type VFSService struct {
	mu sync.Mutex

	wg              sync.WaitGroup
	config_obj      *config_proto.Config
	download_cancel func()
	list_cancel     func()
	logger          *logging.LogContext
}

func (self *VFSService) Close() {
	// Wait for orderly shutdown.
	self.list_cancel()
	self.download_cancel()
	self.wg.Wait()
}

func (self *VFSService) Start() error {
	self.logger.Info("Starting VFS writing service.")

	cancel, err := watchForFlowCompletion(self.config_obj, self.wg, "System.VFS.ListDirectory",
		self.ProcessListDirectory)
	if err != nil {
		return err
	}
	self.list_cancel = cancel

	cancel, err = watchForFlowCompletion(self.config_obj, self.wg, "System.VFS.DownloadFile",
		self.ProcessDownloadFile)
	if err != nil {
		return err
	}
	self.download_cancel = cancel

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
			"flow_id=basename(path=Flow.Urn), " +
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
			utils.GetVFSDownloadInfoPath(client_id, Accessor, Path),
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
		"flow_id=basename(path=Flow.Urn), client_id=ClientId)")
	if err != nil {
		panic(err)
	}

	var rows []vfilter.Row
	current_vfs_path := ""

	for row := range vql.Eval(ctx, sub_scope) {
		full_path := vql_subsystem.GetStringFromRow(scope, row, "_FullPath")
		accessor := vql_subsystem.GetStringFromRow(scope, row, "_Accessor")

		vfs_path := getVfsPath(full_path, accessor)

		// This row does not belong in the current
		// collection - flush the collection and start
		// a new one.
		if path.Dir(vfs_path) != current_vfs_path ||
			// Do not let our memory footprint
			// grow without bounds.
			len(rows) > 100000 {

			// current_vfs_path == "" represents the first
			// collection before the first row is
			// processed.
			if current_vfs_path != "" {
				err := self.flush_state(
					sub_scope, ts, client_id,
					flow_id,
					current_vfs_path, rows)
				if err != nil {
					return
				}
				rows = nil
			}
			current_vfs_path = path.Dir(vfs_path)
		}
		rows = append(rows, row)
	}

	err = self.flush_state(sub_scope, ts, client_id, flow_id,
		current_vfs_path, rows)
	if err != nil {
		self.logger.Error("Unable to save directory: %v", err)
		return
	}
}

// Flush the current state into the database and clear it for the next directory.
func (self *VFSService) flush_state(scope *vfilter.Scope,
	timestamp uint64, client_id, flow_id, vfs_path string,
	rows []vfilter.Row) error {
	if len(rows) == 0 {
		return nil
	}

	serialized, err := json.Marshal(rows)
	if err != nil {
		return errors.WithStack(err)
	}

	urn := urns.BuildURN("clients", client_id, "vfs", vfs_path)

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

// The inverse of getClientPath()
func getVfsPath(client_path string, accessor string) string {
	prefix := "/file"
	switch accessor {
	case "reg":
		prefix = "/registry"
	case "ntfs":
		prefix = "/ntfs"
	}

	return prefix + utils.Normalize_windows_path(client_path)
}

func startVFSService(config_obj *config_proto.Config) (
	*VFSService, error) {
	vfs_service := &VFSService{
		config_obj: config_obj,
		logger:     logging.GetLogger(config_obj, &logging.FrontendComponent),
	}

	return vfs_service, vfs_service.Start()
}
