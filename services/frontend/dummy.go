package frontend

import (
	"context"
	"net/url"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

type DummyFrontendService struct{}

func (self DummyFrontendService) GetMinionCount() int {
	return 0
}

func (self DummyFrontendService) GetMasterAPIClient(
	ctx context.Context) (api_proto.APIClient, func() error, error) {
	return nil, nil, utils.NotImplementedError
}

func (self DummyFrontendService) GetBaseURL(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return nil, utils.NotImplementedError
}

func (self DummyFrontendService) GetPublicUrl(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return nil, utils.NotImplementedError
}

func (self DummyFrontendService) ChargeHouseKeepingOp(
	ctx context.Context) {
}

func (self DummyFrontendService) GetHousekeepSlot(
	ctx context.Context) (func(), error) {
	return func() {}, nil
}
