package config

import (
	"fmt"
	"regexp"
	"strings"

	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Ensures client config is valid, fills in defaults for missing values etc.
func ValidateClientConfig(config_obj *config_proto.Config) error {
	migrate(config_obj)

	// Ensure we have all the sections that are required.
	if config_obj.Client == nil {
		return errors.New("No Client config")
	}

	if config_obj.Client.CaCertificate == "" {
		return errors.New("No Client.ca_certificate configured")
	}

	if config_obj.Client.Nonce == "" {
		return errors.New("No Client.nonce configured")
	}

	if config_obj.Client.ServerUrls == nil {
		return errors.New("No Client.server_urls configured")
	}

	_, err := WritebackLocation(config_obj)
	if err != nil {
		return err
	}

	// Add defaults
	if config_obj.Logging == nil {
		config_obj.Logging = &config_proto.LoggingConfig{}
	}

	// If no local buffer is specified make an in memory one.
	if config_obj.Client.LocalBuffer == nil {
		config_obj.Client.LocalBuffer = &config_proto.RingBufferConfig{
			MemorySize: 50 * 1024 * 1024,
		}
	}

	if config_obj.Client.MaxPoll == 0 {
		config_obj.Client.MaxPoll = 60 // One minute
	}

	if config_obj.Client.PinnedServerName == "" {
		config_obj.Client.PinnedServerName = "VelociraptorServer"
	}

	if config_obj.Client.MaxUploadSize == 0 {
		config_obj.Client.MaxUploadSize = 5242880
	}

	config_obj.Version = GetVersion()
	config_obj.Client.Version = config_obj.Version

	for _, url := range config_obj.Client.ServerUrls {
		if !strings.HasSuffix(url, "/") {
			return errors.New(
				"Configuration Client.server_urls must end with /")
		}
	}

	return nil
}

func ValidateAutoexecConfig(config_obj *config_proto.Config) error {
	return nil
}

// Ensures server config is valid, fills in defaults for missing values etc.
func ValidateFrontendConfig(config_obj *config_proto.Config) error {
	// Check for older version.
	migrate(config_obj)

	if config_obj.API == nil {
		return errors.New("No API config")
	}
	if config_obj.GUI == nil {
		return errors.New("No GUI config")
	}
	if config_obj.GUI.GwCertificate == "" {
		return errors.New("No GUI.gw_certificate config")
	}
	if config_obj.GUI.GwPrivateKey == "" {
		return errors.New("No GUI.gw_private_key config")
	}
	if config_obj.Frontend == nil {
		return errors.New("No Frontend config")
	}
	if config_obj.Frontend.Hostname == "" {
		return errors.New("No Frontend.hostname config")
	}
	if config_obj.Frontend.Certificate == "" {
		return errors.New("No Frontend.certificate config")
	}
	if config_obj.Frontend.PrivateKey == "" {
		return errors.New("No Frontend.private_key config")
	}
	if config_obj.Datastore == nil {
		return errors.New("No Datastore config")
	}

	// Fill defaults for optional sections
	if config_obj.Frontend.Resources == nil {
		config_obj.Frontend.Resources = &config_proto.FrontendResourceControl{}
	}

	// Set default resource controls.
	resources := config_obj.Frontend.Resources
	if resources.ExpectedClients == 0 {
		resources.ExpectedClients = 10000
	}

	// Maximum sustained QPS before load shedding.
	if resources.ConnectionsPerSecond == 0 {
		resources.ConnectionsPerSecond = 100
	}

	if resources.ConnectionsPerSecond > 1000 {
		resources.ConnectionsPerSecond = 1000
	}

	if resources.NotificationsPerSecond == 0 {
		resources.NotificationsPerSecond = 10
	}

	if config_obj.API.PinnedGwName == "" {
		config_obj.API.PinnedGwName = "GRPC_GW"
	}

	return nil
}

func ValidateDatastoreConfig(config_obj *config_proto.Config) error {
	if config_obj.Datastore == nil {
		return errors.New("No Datastore config")
	}

	// If mysql connection params are specified we create
	// a mysql_connection_string
	if config_obj.Datastore.MysqlConnectionString == "" &&
		(config_obj.Datastore.MysqlDatabase != "" ||
			config_obj.Datastore.MysqlServer != "" ||
			config_obj.Datastore.MysqlUsername != "" ||
			config_obj.Datastore.MysqlPassword != "") {
		config_obj.Datastore.MysqlConnectionString = fmt.Sprintf(
			"%s:%s@tcp(%s)/%s",
			config_obj.Datastore.MysqlUsername,
			config_obj.Datastore.MysqlPassword,
			config_obj.Datastore.MysqlServer,
			config_obj.Datastore.MysqlDatabase)
	}

	// On windows we require file locations to include a drive
	// letter.
	if config_obj.ServerType == "windows" {
		path_regex := regexp.MustCompile("^[a-zA-Z]:")
		path_check := func(parameter, value string) error {
			if !path_regex.MatchString(value) {
				return errors.New(fmt.Sprintf(
					"%s must have a drive letter.",
					parameter))
			}
			if strings.Contains(value, "/") {
				return errors.New(fmt.Sprintf(
					"%s can not contain / path separators on windows.",
					parameter))
			}
			return nil
		}

		err := path_check("Datastore.Locations",
			config_obj.Datastore.Location)
		if err != nil {
			return err
		}
		err = path_check("Datastore.Locations",
			config_obj.Datastore.FilestoreDirectory)
		if err != nil {
			return err
		}
	}

	return nil
}
