package remote

import (
	"os"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

func (self *RPCFileStore) StatFile(filename api.FSPathSpec) (api.FileInfo, error) {

	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(self.config_obj), utils.TimeoutError)
	defer cancel()

	// Make the call as the superuser
	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx,
		grpc_client.SuperUser, self.config_obj)
	if err != nil {
		return nil, err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	grpc_result, err := conn.FSStat(ctx,
		&api_proto.DataRequest{
			Pathspec: &api_proto.DSPathSpec{
				Components: filename.Components(),
				PathType:   int64(filename.Type()),
			},
			OrgId: self.config_obj.OrgId,
		})
	if err != nil {
		return nil, utils.StatusToError(err)
	}

	urn := path_specs.NewUnsafeDatastorePath(grpc_result.Components...).
		SetType(api.PathType(grpc_result.Type))

	result := &vtesting.MockFileInfo{
		Name_:     urn.Base(),
		PathSpec_: urn.AsFilestorePath(),
		FullPath_: urn.String(),
		Mode_:     os.FileMode(grpc_result.Mode),
		Size_:     int64(grpc_result.Size),
	}

	return result, nil
}
