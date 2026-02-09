package config

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func deprecated(config_obj *config_proto.Config, name string) {
	logging.Prelog("Config contains deprecated field %v", name)
}

// Migrate from pre 0.4.2 config files.
func migrate_0_4_2(config_obj *config_proto.Config) {
	if config_obj.Frontend != nil && config_obj.AutocertDomain != "" {
		deprecated(config_obj, "autocert_domain")
		config_obj.Frontend.Hostname = config_obj.AutocertDomain
		config_obj.AutocertDomain = ""
	}
	if config_obj.ServerType != "" {
		config_obj.ServerType = "linux"
	}
	if config_obj.Client != nil {
		local_buffer := config_obj.Client.LocalBuffer
		if local_buffer != nil && local_buffer.Filename != "" {
			deprecated(config_obj, "Client.local_buffer.filename")
			local_buffer.Filename = ""
		}
	}

	if config_obj.Frontend != nil &&
		config_obj.Frontend.Certificate != "" {
		if config_obj.Frontend.PublicPath != "" {
			deprecated(config_obj, "Frontend.public_path")
			config_obj.Frontend.PublicPath = ""
		}

		if config_obj.Frontend.DynDns != nil {
			if config_obj.Frontend.DynDns.Hostname != "" {
				deprecated(config_obj, "config_obj.Frontend.dyn_dns.hostname")
				config_obj.Frontend.Hostname = config_obj.Frontend.DynDns.Hostname
				config_obj.Frontend.DynDns.Hostname = ""
			}
		}

		if config_obj.Frontend.Hostname == "" {
			logging.Prelog("Invalid config: New field Frontend.hostname is missing!")

			if config_obj.Client != nil {
				for _, url := range config_obj.Client.ServerUrls {
					re := regexp.MustCompile(`https://([^:/]+)`)
					matches := re.FindStringSubmatch(url)
					if len(matches) > 1 {
						config_obj.Frontend.Hostname = matches[1]
					}
				}
			}

			if config_obj.Frontend.Hostname == "" {
				panic("Unable to deduce the Frontend.hostname")
			}
			logging.Prelog("Guessing Frontend.hostname from Client.server_urls: %v",
				config_obj.Frontend.Hostname)
		}
		if config_obj.ObfuscationNonce == "" {
			sha_sum := sha256.New()
			_, _ = sha_sum.Write([]byte(config_obj.Frontend.PrivateKey))
			config_obj.ObfuscationNonce = hex.EncodeToString(sha_sum.Sum(nil))
		}
	}
}

func migrate_0_4_6(config_obj *config_proto.Config) {
	// We need to migrate old authentication information into an
	// authenticator protobuf.
	if config_obj.GUI != nil &&
		config_obj.GUI.Authenticator == nil {
		gui := config_obj.GUI

		auther := &config_proto.Authenticator{Type: "Basic"}
		if config_obj.GUI.GoogleOauthClientId != "" {
			auther.Type = "Google"
			auther.OauthClientId = gui.GoogleOauthClientId
			auther.OauthClientSecret = gui.GoogleOauthClientSecret

			// Clear the old values
			gui.GoogleOauthClientId = ""
			gui.GoogleOauthClientSecret = ""

		} else if config_obj.GUI.SamlCertificate != "" {
			auther.Type = "SAML"
			auther.SamlCertificate = gui.SamlCertificate
			auther.SamlPrivateKey = gui.SamlPrivateKey
			auther.SamlIdpMetadataUrl = gui.SamlIdpMetadataUrl
			auther.SamlRootUrl = gui.SamlRootUrl
			auther.SamlUserAttribute = gui.SamlUserAttribute

			// Clear the old values
			gui.SamlCertificate = ""
			gui.SamlPrivateKey = ""
			gui.SamlIdpMetadataUrl = ""
			gui.SamlRootUrl = ""
			gui.SamlUserAttribute = ""
		}

		config_obj.GUI.Authenticator = auther
	}

	if config_obj.Datastore != nil {
		if config_obj.Datastore.Implementation == "FileBaseDataStore" &&
			config_obj.Datastore.Location == "" {
			config_obj.Datastore.Location = config_obj.Datastore.FilestoreDirectory
		}
	}
}

func migrate_0_5_6(config_obj *config_proto.Config) {
	if config_obj.Logging != nil {
		default_rotator := &config_proto.LoggingRetentionConfig{
			RotationTime: config_obj.Logging.RotationTime,
			MaxAge:       config_obj.Logging.MaxAge,
		}

		if config_obj.Logging.Debug == nil {
			config_obj.Logging.Debug = default_rotator
		}

		if config_obj.Logging.Info == nil {
			config_obj.Logging.Debug = default_rotator
		}

		if config_obj.Logging.Error == nil {
			config_obj.Logging.Debug = default_rotator
		}
	}

	if config_obj.Frontend != nil {
		if config_obj.Frontend.Resources == nil {
			config_obj.Frontend.Resources = &config_proto.FrontendResourceControl{
				Concurrency:         config_obj.Frontend.Concurrency,
				MaxUploadSize:       config_obj.Frontend.MaxUploadSize,
				ExpectedClients:     config_obj.Frontend.ExpectedClients,
				PerClientUploadRate: config_obj.Frontend.PerClientUploadRate,
				GlobalUploadRate:    config_obj.Frontend.GlobalUploadRate,
				ClientEventMaxWait:  config_obj.Frontend.ClientEventMaxWait,
			}

			config_obj.Frontend.Concurrency = 0
			config_obj.Frontend.MaxUploadSize = 0
			config_obj.Frontend.ExpectedClients = 0
			config_obj.Frontend.PerClientUploadRate = 0
			config_obj.Frontend.GlobalUploadRate = 0
			config_obj.Frontend.ClientEventMaxWait = 0
		}

		// Update all the extra frontends to use the same resources.
		for _, fe := range config_obj.ExtraFrontends {
			fe.Resources = config_obj.Frontend.Resources
		}
	}
}

func migrate_0_6_1(config_obj *config_proto.Config) {
	// We require the datastore location to have no trailing path
	// separators.
	if config_obj.Datastore != nil {
		config_obj.Datastore.Location = strings.TrimSuffix(
			config_obj.Datastore.Location, "\\")

		config_obj.Datastore.Location = strings.TrimSuffix(
			config_obj.Datastore.Location, "/")

		config_obj.Datastore.FilestoreDirectory = strings.TrimSuffix(
			config_obj.Datastore.FilestoreDirectory, "\\")

		config_obj.Datastore.FilestoreDirectory = strings.TrimSuffix(
			config_obj.Datastore.FilestoreDirectory, "/")
	}

	if config_obj.Defaults == nil {
		config_obj.Defaults = &config_proto.Defaults{}
	}

	if config_obj.Defaults.HuntExpiryHours == 0 {
		config_obj.Defaults.HuntExpiryHours = 24 * 7
	}

	if config_obj.Defaults.NotebookCellTimeoutMin == 0 {
		config_obj.Defaults.NotebookCellTimeoutMin = 10
	}
}

func migrate_0_7_0(config_obj *config_proto.Config) {
	// Check for defaults:
	if config_obj.Defaults == nil {
		config_obj.Defaults = &config_proto.Defaults{}
	}

	if config_obj.Defaults.NotebookNumberOfLocalWorkers == 0 {
		config_obj.Defaults.NotebookNumberOfLocalWorkers = 5
	}
}

func migrate_0_7_5(config_obj *config_proto.Config) {
	if config_obj.Security == nil {
		config_obj.Security = &config_proto.Security{}
	}

	if len(config_obj.Defaults.AllowedPlugins) > 0 {
		config_obj.Security.AllowedPlugins = append(
			config_obj.Security.AllowedPlugins,
			config_obj.Defaults.AllowedPlugins...)
		config_obj.Defaults.AllowedPlugins = nil
	}

	if len(config_obj.Defaults.AllowedFunctions) > 0 {
		config_obj.Security.AllowedFunctions = append(
			config_obj.Security.AllowedFunctions,
			config_obj.Defaults.AllowedFunctions...)
		config_obj.Defaults.AllowedFunctions = nil
	}

	if len(config_obj.Defaults.AllowedAccessors) > 0 {
		config_obj.Security.AllowedAccessors = append(
			config_obj.Security.AllowedAccessors,
			config_obj.Defaults.AllowedAccessors...)
		config_obj.Defaults.AllowedAccessors = nil
	}

	if len(config_obj.Defaults.DeniedPlugins) > 0 {
		config_obj.Security.DeniedPlugins = append(
			config_obj.Security.DeniedPlugins,
			config_obj.Defaults.DeniedPlugins...)
		config_obj.Defaults.DeniedPlugins = nil
	}

	if len(config_obj.Defaults.DeniedFunctions) > 0 {
		config_obj.Security.DeniedFunctions = append(
			config_obj.Security.DeniedFunctions,
			config_obj.Defaults.DeniedFunctions...)
		config_obj.Defaults.DeniedFunctions = nil
	}

	if len(config_obj.Defaults.DeniedAccessors) > 0 {
		config_obj.Security.DeniedAccessors = append(
			config_obj.Security.DeniedAccessors,
			config_obj.Defaults.DeniedAccessors...)
		config_obj.Defaults.DeniedAccessors = nil
	}

	if len(config_obj.Defaults.LockdownDeniedPermissions) > 0 {
		config_obj.Security.LockdownDeniedPermissions = append(
			config_obj.Security.LockdownDeniedPermissions,
			config_obj.Defaults.LockdownDeniedPermissions...)
		config_obj.Defaults.LockdownDeniedPermissions = nil
	}

	if config_obj.Defaults.CertificateValidityDays > 0 {
		config_obj.Security.CertificateValidityDays =
			config_obj.Defaults.CertificateValidityDays
	}

	if config_obj.Defaults.DisableInventoryServiceExternalAccess {
		config_obj.Security.DisableInventoryServiceExternalAccess = true
	}

}

func migrate(config_obj *config_proto.Config) {
	migrate_0_4_2(config_obj)
	migrate_0_4_6(config_obj)
	migrate_0_5_6(config_obj)
	migrate_0_6_1(config_obj)
	migrate_0_7_0(config_obj)
	migrate_0_7_5(config_obj)
}
