package startup

import (
	"context"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
)

// Start minimal services for tools.
func StartToolServices(
	ctx context.Context,
	config_obj *config_proto.Config) (*services.Service, error) {
	sm := services.NewServiceManager(ctx, config_obj)
	_, err := orgs.NewOrgManager(sm.Ctx, sm.Wg, config_obj)
	if err != nil {
		return sm, err
	}

	return sm, nil
}
