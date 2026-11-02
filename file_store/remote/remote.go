package remote

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

type RPCFileStore struct {
	ctx        context.Context
	config_obj *config_proto.Config
}

func NewRemoteFileStore(
	ctx context.Context,
	config_obj *config_proto.Config) (api.FileStore, error) {
	return &RPCFileStore{
		ctx:        ctx,
		config_obj: config_obj,
	}, nil
}
