package vfs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	ErrNotFound     = errors.New("file not found")
	ErrNotAvailable = errors.New("File content not available")
)

type VFSFileSystemAccessor struct {
	ctx                 context.Context
	client_id           string
	config_obj          *config_proto.Config
	file_store_accessor accessors.FileSystemAccessor
}

func (self VFSFileSystemAccessor) New(
	scope vfilter.Scope) (accessors.FileSystemAccessor, error) {

	// Check we have permission to open files.
	err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
	if err != nil {
		return nil, err
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil, errors.New("vfs accessor: can only run on the server")
	}

	client_id, pres := scope.Resolve("ClientId")
	if !pres {
		return nil, errors.New("vfs accessor: ClientId does not exist in the scope")
	}

	client_id_str, ok := client_id.(string)
	if !ok {
		return nil, errors.New("vfs accessor: ClientId does not exist in the scope")
	}

	accessor, err := accessors.GetAccessor("fs", scope)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = vql_subsystem.GetRootScope(scope).AddDestructor(cancel)
	if err != nil {
		return nil, err
	}

	return &VFSFileSystemAccessor{
		ctx:                 ctx,
		client_id:           client_id_str,
		file_store_accessor: accessor,
		config_obj:          config_obj,
	}, nil
}

func (self VFSFileSystemAccessor) Lstat(filename string) (
	accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.LstatWithOSPath(full_path)
}

func (self VFSFileSystemAccessor) LstatWithOSPath(filename *accessors.OSPath) (
	accessors.FileInfo, error) {
	vfs_service, err := services.GetVFSService(self.config_obj)
	if err != nil {
		return nil, err
	}

	res, err := vfs_service.ListDirectoryFiles(self.ctx,
		self.config_obj, &api_proto.GetTableRequest{
			Rows:          1000,
			ClientId:      self.client_id,
			VfsComponents: filename.Dirname().Components,
		})
	if err != nil {
		return nil, err
	}
	// Find the row that matches this filename
	for _, r := range res.Rows {
		if len(r.Cell) < 12 {
			continue
		}

		if r.Cell[5] == filename.Basename() {
			return rowCellToFSInfo(r.Cell)
		}
	}

	return nil, ErrNotFound
}

func (self VFSFileSystemAccessor) ParsePath(path string) (
	*accessors.OSPath, error) {
	return accessors.NewGenericOSPath(path)
}

func (self VFSFileSystemAccessor) ReadDir(filename string) (
	[]accessors.FileInfo, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.ReadDirWithOSPath(full_path)
}

func (self VFSFileSystemAccessor) ReadDirWithOSPath(
	filename *accessors.OSPath) (
	[]accessors.FileInfo, error) {

	vfs_service, err := services.GetVFSService(self.config_obj)
	if err != nil {
		return nil, err
	}

	res, err := vfs_service.ListDirectoryFiles(self.ctx,
		self.config_obj, &api_proto.GetTableRequest{
			Rows:          1000,
			ClientId:      self.client_id,
			VfsComponents: filename.Components,
		})
	if err != nil {
		return nil, err
	}

	result := []accessors.FileInfo{}
	for _, r := range res.Rows {
		if len(r.Cell) < 12 {
			continue
		}

		fs_info, err := rowCellToFSInfo(r.Cell)
		if err != nil {
			continue
		}
		result = append(result, fs_info)
	}

	return result, nil
}

func (self VFSFileSystemAccessor) Open(filename string) (
	accessors.ReadSeekCloser, error) {
	full_path, err := self.ParsePath(filename)
	if err != nil {
		return nil, err
	}

	return self.OpenWithOSPath(full_path)
}

func (self VFSFileSystemAccessor) OpenWithOSPath(filename *accessors.OSPath) (
	accessors.ReadSeekCloser, error) {

	vfs_service, err := services.GetVFSService(self.config_obj)
	if err != nil {
		return nil, err
	}

	res, err := vfs_service.ListDirectoryFiles(self.ctx,
		self.config_obj, &api_proto.GetTableRequest{
			Rows:          1000,
			ClientId:      self.client_id,
			VfsComponents: filename.Dirname().Components,
		})
	if err != nil {
		return nil, err
	}

	// Find the row that matches this filename
	for _, r := range res.Rows {
		if len(r.Cell) < 12 {
			continue
		}

		if r.Cell[5] == filename.Basename() {
			// Check if it has a download link
			record := &flows_proto.VFSDownloadInfo{}
			err = json.Unmarshal([]byte(r.Cell[0]), record)
			if err != nil || record.Name == "" {
				return nil, ErrNotAvailable
			}

			return self.file_store_accessor.OpenWithOSPath(
				accessors.MustNewFileStorePath("").Append(record.Components...))
		}
	}

	return nil, ErrNotFound
}

func rowCellToFSInfo(cell []string) (accessors.FileInfo, error) {
	components := []string{}
	err := json.Unmarshal([]byte(cell[2]), &components)
	if err != nil {
		return nil, err
	}

	size := int64(0)
	_ = json.Unmarshal([]byte(cell[6]), &size)

	is_dir := false
	if len(cell[7]) > 1 && cell[7][0] == 'd' {
		is_dir = true
	}

	// The Accessor + components is the path of the item
	path := accessors.MustNewGenericOSPath(cell[3]).Append(components...)
	fs_info := &accessors.VirtualFileInfo{
		Path:   path,
		IsDir_: is_dir,
		Size_:  size,
		Data_:  ordereddict.NewDict(),
	}

	// The download pointer allows us to fetch the file itself.
	record := &flows_proto.VFSDownloadInfo{}
	err = json.Unmarshal([]byte(cell[0]), record)
	if err == nil {
		fs_info.Data_.Set("DownloadInfo", record)
	}

	return fs_info, nil
}

func init() {
	accessors.Register("vfs", &VFSFileSystemAccessor{}, `Access client's VFS filesystem on the server.`)
}
