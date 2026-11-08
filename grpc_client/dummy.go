package grpc_client

import (
	"context"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type DummyGRPCAPIClient struct {
	mu sync.Mutex
	APIClientFactory
}

func (self *DummyGRPCAPIClient) GetAPIClient(
	ctx context.Context,
	identity CallerIdentity,
	config_obj *config_proto.Config) (
	api_proto.APIClient, func() error, error) {
	self.mu.Lock()
	client := self.APIClientFactory
	if client == nil {
		new_client := NewDirectConnectionClient()
		self.APIClientFactory = new_client
		client = new_client
	}
	self.mu.Unlock()

	return client.GetAPIClient(ctx, identity, config_obj)
}
