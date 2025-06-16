package config

import config_proto "www.velocidex.com/golang/velociraptor/config/proto"

// The client config is a reducted version of the server config with
// sensitive values removed.
func GetClientConfig(config_obj *config_proto.Config) *config_proto.Config {
	// Copy only settings relevant to the client from the main config.
	client_config := &config_proto.Config{
		Version: config_obj.Version,
		Client:  config_obj.Client,
	}

	return client_config
}
