package services

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func GetLSPServer(config_obj *config_proto.Config) (LSPServer, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).LSPServer()
}

type LSPServer interface {
	LSP(ctx context.Context,
		in *api_proto.LSPRequest) (*api_proto.LSPResponse, error)
}
