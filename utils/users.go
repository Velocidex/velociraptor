package utils

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
)

func GetSuperuserName(
	config_obj *config_proto.Config) string {
	if config_obj == nil ||
		config_obj.Client == nil ||
		config_obj.Client.PinnedServerName == "" {
		return constants.PinnedServerName
	}

	return config_obj.Client.PinnedServerName
}

// The name of the gateway certificate. This is specified in the
// GUI.gw_certificate and is populated by the sanity service.
func GetGatewayName(
	config_obj *config_proto.Config) string {
	if config_obj == nil ||
		config_obj.API == nil ||
		config_obj.API.PinnedGwName == "" {
		return constants.PinnedGwName
	}

	return config_obj.API.PinnedGwName
}
