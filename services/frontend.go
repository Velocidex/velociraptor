package services

import (
	"context"
	"fmt"
	"net/url"
	"sync/atomic"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The frontend service manages load balancing between multiple
// frontends. Velociraptor clients may be redirected between active
// frontends to spread the load between them.

var (
	FrontendIsMaster          = fmt.Errorf("FrontendIsMaster: %w", utils.NotFoundError)
	NotRunningInFrontendError = utils.Wrap(utils.InvalidConfigError,
		"Command not available when running without a frontend service. To perform administrative tasks on the command line, connect to the server using the API https://docs.velociraptor.app/docs/server_automation/server_api/")

	// A bypass for RequireFrontend() set during tests.
	AllowFrontendPlugins = atomic.Bool{}
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

	// Calculates the Base URL to the top of the app
	GetBaseURL(config_obj *config_proto.Config) (res *url.URL, err error)

	// The URL to the App.html itself
	GetPublicUrl(config_obj *config_proto.Config) (res *url.URL, err error)

	SetGlobalMessage(message *api_proto.GlobalUserMessage)
	GetGlobalMessages() []*api_proto.GlobalUserMessage
}

// Are we running on the master node?
func IsMaster(config_obj *config_proto.Config) bool {
	if config_obj.Frontend != nil {
		return !config_obj.Frontend.IsMinion
	}
	return true
}

func IsClient(config_obj *config_proto.Config) bool {
	return config_obj.Frontend == nil
}

func IsMinion(config_obj *config_proto.Config) bool {
	return !IsMaster(config_obj)
}

/*
RequireFrontend: Ensure we are running inside the server.

This is used by VQL plugins that change server state to make sure the
VQL query is running inside a valid frontend. Since VQL queries can
run with the `velociraptor query` command it is possible they are just
running on the same server as Velociraptor (and therefore the data
store is still visible) but it is important to make sure the datastore
is not modified outside the proper frontend process.

This is because many services are now caching data in memory and
changing the underlying data stored will not be immediately visible to
them.

As an exception we allow tests to bypass this check because they are
not always running a frontend service.
*/
func RequireFrontend() error {
	if AllowFrontendPlugins.Load() {
		return nil
	}

	org_manager, err := GetOrgManager()
	if err != nil {
		return err
	}

	_, err = org_manager.Services(ROOT_ORG_ID).FrontendManager()
	if err != nil {
		return NotRunningInFrontendError
	}
	return nil
}

func GetNodeName(frontend_config *config_proto.FrontendConfig) string {
	if frontend_config == nil {
		return "-"
	}
	return fmt.Sprintf("%s-%d", frontend_config.Hostname,
		frontend_config.BindPort)
}
