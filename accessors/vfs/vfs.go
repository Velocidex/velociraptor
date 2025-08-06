package vfs

import (
	"context"
	"errors"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	ErrNotFound     = errors.New("file not found")
	ErrNotAvailable = errors.New("File content not available")
	ErrInvalidRow   = errors.New("Stored row is invalid")
)

type VFSFileSystemAccessor struct {
	ctx                 context.Context
	client_id           string
	config_obj          *config_proto.Config
	file_store_accessor accessors.FileSystemAccessor
}

func (self VFSFileSystemAccessor) New(
	scope vfilter.Scope) (accessors.FileSystemAccessor, error) {

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

func (self VFSFileSystemAccessor) Describe() *accessors.AccessorDescriptor {
	return &accessors.AccessorDescriptor{
		Name:        "vfs",
		Description: `Access client's VFS filesystem on the server.`,
		Permissions: []acls.ACL_PERMISSION{acls.READ_RESULTS},
	}
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
		var row []interface{}
		_ = json.Unmarshal([]byte(r.Json), &row)
		if len(row) < 12 {
			continue
		}

		name, ok := row[5].(string)
		if ok && name == filename.Basename() {
			return rowCellToFSInfo(row)
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
		var row []interface{}
		_ = json.Unmarshal([]byte(r.Json), &row)
		if len(row) < 12 {
			continue
		}

		fs_info, err := rowCellToFSInfo(row)
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
		var row []interface{}
		_ = json.Unmarshal([]byte(r.Json), &row)
		if len(row) < 12 {
			continue
		}

		name, ok := row[5].(string)
		if !ok {
			continue
		}

		if name == filename.Basename() {
			// Check if it has a download link
			record := &flows_proto.VFSDownloadInfo{}
			err = utils.ParseIntoProtobuf(row[0], record)
			if err != nil || record.Name == "" {
				return nil, ErrNotAvailable
			}

			return self.file_store_accessor.OpenWithOSPath(
				accessors.MustNewFileStorePath("").Append(record.Components...))
		}
	}

	return nil, ErrNotFound
}

func rowCellToFSInfo(cell []interface{}) (accessors.FileInfo, error) {
	components := utils.ConvertToStringSlice(cell[2])
	if len(components) == 0 {
		return nil, ErrInvalidRow
	}

	size, ok := utils.ToInt64(cell[6])
	if !ok {
		return nil, ErrInvalidRow
	}

	mode, ok := cell[7].(string)
	if !ok {
		return nil, ErrInvalidRow
	}

	is_dir := len(mode) > 1 && mode[0] == 'd'

	// The Accessor + components is the path of the item
	ospath, ok := cell[3].(string)
	if !ok {
		return nil, ErrInvalidRow
	}

	path := accessors.MustNewGenericOSPath(ospath).Append(components...)
	fs_info := &accessors.VirtualFileInfo{
		Path:   path,
		IsDir_: is_dir,
		Size_:  size,
		Data_:  ordereddict.NewDict(),
	}

	// The download pointer allows us to fetch the file itself.
	record := &flows_proto.VFSDownloadInfo{}
	err := utils.ParseIntoProtobuf(cell[0], record)
	if err == nil {
		fs_info.Data_.Set("DownloadInfo", record)
	}

	return fs_info, nil
}

func init() {
	accessors.Register(&VFSFileSystemAccessor{})
}
