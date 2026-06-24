package config

import config_proto "www.velocidex.com/golang/velociraptor/config/proto"

// The client config is a reduced version of the server config with
// sensitive values removed.
func GetClientConfig(config_obj *config_proto.Config) *config_proto.Config {
	// Copy only settings relevant to the client from the main config.
	client_config := &config_proto.Config{
		Version:  config_obj.Version,
		Client:   config_obj.Client,
		Autoexec: config_obj.Autoexec,

		// The below are added at runtime..
		Remappings: config_obj.Remappings, // --remap flag.
		Verbose:    config_obj.Verbose,    // -v flag
		DebugMode:  config_obj.DebugMode,  // --debug flag

		// Logging is populated from the client specific section.
		Logging: config_obj.Client.Logging,
	}

	return client_config
}

// Produce a minima configuration for the client. This function is
// used to prepare a client config from the server config file
// (e.g. for packaging).
func StripClientConfig(config_obj *config_proto.Config) *config_proto.Config {
	// Copy only settings relevant to the client from the main config.
	client_config := &config_proto.Config{
		Version:  config_obj.Version,
		Client:   config_obj.Client,
		Autoexec: config_obj.Autoexec,
	}

	return client_config
}

// Determine if the binary is running as a frontend. At a minimum we
// need a valid Frontend and Datastore sections.
func IsFrontend(config_obj *config_proto.Config) bool {
	return config_obj.Datastore != nil &&
		config_obj.Frontend != nil
}

// Determine if the binary is running as a client. At a minimum we
// need a valid Client section.
func IsClient(config_obj *config_proto.Config) bool {
	return config_obj.Client != nil
}
