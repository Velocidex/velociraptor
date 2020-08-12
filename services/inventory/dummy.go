package inventory

import (
	"context"

	"github.com/pkg/errors"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

type Dummy struct{}

func (self Dummy) Get() *artifacts_proto.ThirdParty {
	return &artifacts_proto.ThirdParty{}
}

func (self Dummy) GetToolInfo(ctx context.Context, config_obj *config_proto.Config,
	tool string) (*artifacts_proto.Tool, error) {
	return nil, errors.New("Not found")
}

func (self Dummy) AddTool(config_obj *config_proto.Config, tool *artifacts_proto.Tool) error {
	return errors.New("Inventory service not available")
}

func (self Dummy) RemoveTool(config_obj *config_proto.Config, tool_name string) error {
	return errors.New("Inventory service not available")
}

func init() {
	services.RegisterInventory(&Dummy{})
}
