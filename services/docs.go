package services

import (
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func GetDocManager(config_obj *config_proto.Config) (DocManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).DocManager()
}

// The doc manager is responsible for searching documentation.
type DocManager interface {
	Search(ctx context.Context,
		query string, start, len int) (*api_proto.DocSearchResponses, error)
}
