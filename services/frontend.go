package services

import (
	"context"
	"fmt"
	"os"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// The frontend service manages load balancing between multiple
// frontends. Velociraptor clients may be redirected between active
// frontends to spread the load between them.

var (
	FrontendIsMaster = os.ErrNotExist
)

func GetFrontendManager(config_obj *config_proto.Config) (
	FrontendManager, error) {
	org_manager, err := GetOrgManager()
	if err != nil {
		return nil, err
	}

	return org_manager.Services(config_obj.OrgId).FrontendManager()
}

type FrontendManager interface {
	GetMinionCount() int

	// Establish a gRPC connection to the master node. If we are
	// running on the master node already then returns a
	// fs.ErrNotExist error. If we fail to connect returns another
	// error.
	GetMasterAPIClient(ctx context.Context) (
		api_proto.APIClient, func() error, error)
}

// Are we running on the master node?
func IsMaster(config_obj *config_proto.Config) bool {
	if config_obj.Frontend != nil {
		return !config_obj.Frontend.IsMinion
	}
	return true
}

func GetNodeName(frontend_config *config_proto.FrontendConfig) string {
	return fmt.Sprintf("%s-%d", frontend_config.Hostname,
		frontend_config.BindPort)
}
