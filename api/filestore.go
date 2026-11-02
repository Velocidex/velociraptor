package api

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ApiServer) VFSGetBuffer(
	ctx context.Context,
	in *api_proto.VFSFileBuffer) (
	*api_proto.VFSFileBuffer, error) {

	defer Instrument("VFSGetBuffer")()

	// Anyone in the org can read the filestore.
	org_config_obj, err := checkDataStorePermissions(
		ctx, in.OrgId, acls.READ_RESULTS)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// If a client id is specified, the path is relative to the
	// client's storage directory, otherwise it is relative to the
	// root of the filestore.
	var pathspec api.FSPathSpec
	if in.ClientId != "" {
		pathspec = paths.NewClientPathManager(
			in.ClientId).FSItem(in.Components)

	} else if len(in.Components) > 0 {
		if in.PathType > 0 {
			pathspec = path_specs.NewUnsafeFilestorePath(in.Components...).SetType(
				api.PathType(in.PathType))
		} else {
			pathspec = path_specs.FromGenericComponentList(in.Components)
		}

	} else {
		return nil, status.Error(codes.InvalidArgument,
			"Invalid pathspec")
	}

	if len(pathspec.Components()) == 0 {
		return nil, Status(self.verbose, utils.InvalidArgError)
	}

	padding := true
	if in.Padding != nil {
		padding = *in.Padding
	}

	result, err := self.vfsGetBuffer(
		org_config_obj, in.ClientId, pathspec, in.Offset, in.Length, padding)

	return result, Status(self.verbose, err)
}

func (self *ApiServer) FSWriteBuffer(
	ctx context.Context,
	in *api_proto.VFSFileBuffer) (*api_proto.FSWriteBufferResponse, error) {

	config_obj, err := checkDataStorePermissions(ctx, in.OrgId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	path_spec := path_specs.NewUnsafeFilestorePath(in.Components...).SetType(
		api.PathType(in.PathType))

	err = file_store_accessor.IsFileAccessible(path_spec)
	if err != nil {
		return nil, err
	}

	if len(path_spec.Components()) == 0 {
		return nil, Status(self.verbose, utils.InvalidArgError)
	}

	file, err := file_store.GetFileStore(config_obj).WriteFile(path_spec)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if in.Truncate {
		file.Truncate()
	}

	if in.UncompressedSize > 0 {
		n, err := file.WriteCompressed(in.Data, in.Offset, int(in.UncompressedSize))
		return &api_proto.FSWriteBufferResponse{
			Written: int64(n),
			Error:   utils.MakeError(err),
		}, nil
	}

	n, err := file.Write(in.Data)
	return &api_proto.FSWriteBufferResponse{
		Written: int64(n),
		Error:   utils.MakeError(err),
	}, nil
}

func (self *ApiServer) FSStat(
	ctx context.Context,
	in *api_proto.DataRequest) (*api_proto.FSFileInfo, error) {

	defer Instrument("FSStat")()

	org_config_obj, err := checkDataStorePermissions(ctx, in.OrgId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	path := in.Pathspec
	if path == nil {
		return nil, Status(self.verbose, utils.InvalidArgError)
	}

	filename := path_specs.NewUnsafeFilestorePath(
		path.Components...).SetType(api.PathType(path.PathType))

	err = file_store_accessor.IsFileAccessible(filename)
	if err != nil {
		return nil, err
	}

	file_store_factory := file_store.GetFileStore(org_config_obj)
	stat, err := file_store_factory.StatFile(filename)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	child_pathspec := stat.PathSpec()
	return &api_proto.FSFileInfo{
		Components: child_pathspec.Components(),
		Type:       int64(child_pathspec.Type()),
		Mode:       uint32(stat.Mode()),
		ModTime:    uint64(stat.ModTime().UnixNano()),
	}, nil
}

func (self *ApiServer) FSDelete(
	ctx context.Context,
	in *api_proto.DataRequest) (*api_proto.DataRequest, error) {

	defer Instrument("FSDelete")()

	org_config_obj, err := checkDataStorePermissions(ctx, in.OrgId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	path := in.Pathspec
	if path == nil {
		return nil, Status(self.verbose, utils.InvalidArgError)
	}

	filename := path_specs.NewUnsafeFilestorePath(
		path.Components...).SetType(api.PathType(path.PathType))

	err = file_store_accessor.IsFileAccessible(filename)
	if err != nil {
		return nil, err
	}

	file_store_factory := file_store.GetFileStore(org_config_obj)
	err = file_store_factory.Delete(filename)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return in, nil
}

// Read the filestore directory
func (self *ApiServer) FSReadDir(
	ctx context.Context,
	in *api_proto.DataRequest) (*api_proto.FSReadDirResponse, error) {

	defer Instrument("FSReadDir")()

	org_config_obj, err := checkDataStorePermissions(ctx, in.OrgId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	path := in.Pathspec
	if path == nil {
		return nil, Status(self.verbose, utils.InvalidArgError)
	}

	filename := path_specs.NewUnsafeFilestorePath(
		path.Components...).SetType(api.PathType(path.PathType))
	file_store_factory := file_store.GetFileStore(org_config_obj)
	children, err := file_store_factory.ListDirectory(filename)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := &api_proto.FSReadDirResponse{}
	for _, child := range children {
		child_pathspec := child.PathSpec()

		result.Items = append(result.Items, &api_proto.FSFileInfo{
			Components: child_pathspec.Components(),
			Type:       int64(child_pathspec.Type()),
			Mode:       uint32(child.Mode()),
			ModTime:    uint64(child.ModTime().UnixNano()),
		})
	}

	return result, nil
}
