package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	"www.velocidex.com/golang/velociraptor/utils"
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

	// Rename fields as needed.
	if config_obj.Client.ServerVersion == nil &&
		config_obj.Client.Version != nil {
		config_obj.Client.ServerVersion = config_obj.Client.Version
		config_obj.Client.Version = nil
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

	if config_obj.Client.MaxUploadSize == 0 {
		config_obj.Client.MaxUploadSize = 5242880
	}

	if config_obj.Client.Crypto != nil {
		allowed_verification_modes := []string{
			"", "PKI", "PKI_OR_THUMBPRINT", "THUMBPRINT_ONLY",
		}
		if !utils.InString(allowed_verification_modes,
			strings.ToUpper(config_obj.Client.Crypto.CertificateVerificationMode)) {
			return fmt.Errorf("Client.Crypto.certificate_verification_mode not valid! Should be one of %v",
				allowed_verification_modes)
		}
	}

	config_obj.Version = GetVersion()

	// The client's config contains the running version of the client
	// itself.
	config_obj.Client.Version = GetVersion()

	// Ensure the writeback service is configured.
	writeback_service := writeback.GetWritebackService()
	writeback, err := writeback_service.GetWriteback(config_obj)
	if err == nil && writeback.InstallTime != 0 {

		// Sync the version InstallTime from the writeback. This way
		// VQL can see that through the config VQL env variable.
		config_obj.Version.InstallTime = writeback.InstallTime
	}

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
		config_obj.API.PinnedGwName = constants.PinnedGwName
	}

	// The server should always update the client part to ensure new
	// clients created from this server will keep the correct server
	// version.
	if config_obj.Client != nil {
		version := GetVersion()

		config_obj.Client.ServerVersion = &config_proto.Version{
			Version:   version.Version,
			BuildTime: version.BuildTime,
			Commit:    version.Commit,
		}

	}

	return nil
}

func ValidateDatastoreConfig(config_obj *config_proto.Config) error {
	if config_obj.Datastore == nil {
		return errors.New("No Datastore config")
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
