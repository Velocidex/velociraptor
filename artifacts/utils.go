package artifacts

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Gets the config from the scope.
func GetConfig(scope *vfilter.Scope) (*config_proto.ClientConfig, bool) {
	scope_config, pres := scope.Resolve(constants.SCOPE_CONFIG)
	if !pres {
		return nil, false
	}

	config, ok := scope_config.(*config_proto.ClientConfig)
	if config == nil {
		return nil, false
	}
	return config, ok
}

func GetServerConfig(scope *vfilter.Scope) (*config_proto.Config, bool) {
	scope_config, pres := scope.Resolve(constants.SCOPE_SERVER_CONFIG)
	if !pres {
		return nil, false
	}

	config, ok := scope_config.(*config_proto.Config)
	return config, ok
}

func GetUploader(scope *vfilter.Scope) (api.Uploader, bool) {
	scope_uploader, pres := scope.Resolve(constants.SCOPE_UPLOADER)
	if !pres {
		return nil, false
	}

	config, ok := scope_uploader.(api.Uploader)
	if utils.IsNil(config) {
		return nil, false
	}

	return config, ok
}

func GetACLManager(scope *vfilter.Scope) (vql_subsystem.ACLManager, bool) {
	scope_manager, pres := scope.Resolve(vql_subsystem.ACL_MANAGER_VAR)
	if !pres {
		return nil, false
	}

	config, ok := scope_manager.(vql_subsystem.ACLManager)
	if utils.IsNil(config) {
		return nil, false
	}

	return config, ok
}
