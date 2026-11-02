package remote

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *RPCFileStore) Delete(filename api.FSPathSpec) error {
	ctx, cancel := utils.WithTimeoutCause(
		self.ctx, utils.RPCTimeout(self.config_obj), utils.TimeoutError)
	defer cancel()

	// Make the call as the superuser
	conn, closer, err := grpc_client.Factory.GetAPIClient(ctx,
		grpc_client.SuperUser, self.config_obj)
	if err != nil {
		return err
	}
	defer func() {
		err1 := closer()
		if err1 != nil && err == nil {
			err = err1
		}
	}()

	_, err = conn.FSDelete(ctx,
		&api_proto.DataRequest{
			Pathspec: &api_proto.DSPathSpec{
				Components: filename.Components(),
				PathType:   int64(filename.Type()),
			},
			OrgId: self.config_obj.OrgId,
		})

	return utils.StatusToError(err)
}
