package inventory

import (
	"context"

	"github.com/pkg/errors"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

type Dummy struct{}

func (self Dummy) Get() *artifacts_proto.ThirdParty {
	return &artifacts_proto.ThirdParty{}
}

func (self Dummy) ProbeToolInfo(name string) (*artifacts_proto.Tool, error) {
	return nil, errors.New("ProbeToolInfo Not implemented")
}

func (self Dummy) GetToolInfo(ctx context.Context,
	config_obj *config_proto.Config, tool string) (*artifacts_proto.Tool, error) {
	return &artifacts_proto.Tool{}, nil
	return nil, errors.New("GetToolInfo Not implemented")
}

func (self Dummy) AddTool(ctx context.Context,
	config_obj *config_proto.Config, tool *artifacts_proto.Tool) error {
	json.Dump(tool)
	return nil
	return errors.New("AddTool Not implemented")
}

func (self Dummy) RemoveTool(config_obj *config_proto.Config, tool_name string) error {
	return errors.New("RemoveTool Not implemented")
}
