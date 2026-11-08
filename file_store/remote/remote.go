package remote

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/grpc_client"
)

type RPCFileStore struct {
	ctx        context.Context
	config_obj *config_proto.Config
	factory    func() (api_proto.APIClient, func() error, error)
}

type RemoteFileStoreOptions struct {
	Factory func() (api_proto.APIClient, func() error, error)
}

func NewRemoteFileStore(
	ctx context.Context,
	config_obj *config_proto.Config,
	opts ...RemoteFileStoreOptions) (api.FileStore, error) {
	var factory func() (api_proto.APIClient, func() error, error)

	for _, opt := range opts {
		if factory == nil {
			factory = opt.Factory
		}
	}

	if factory == nil {
		factory = func() (api_proto.APIClient, func() error, error) {
			return grpc_client.Factory.GetAPIClient(ctx,
				grpc_client.SuperUser, config_obj)
		}
	}

	return &RPCFileStore{
		ctx:        ctx,
		config_obj: config_obj,
		factory:    factory,
	}, nil
}
