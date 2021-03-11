package config

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"

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

	if config_obj.Frontend != nil {
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

			for _, url := range config_obj.Client.ServerUrls {
				re := regexp.MustCompile(`https://([^:/]+)`)
				matches := re.FindStringSubmatch(url)
				if len(matches) > 1 {
					config_obj.Frontend.Hostname = matches[1]
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
		}
	}
}

func migrate(config_obj *config_proto.Config) {
	migrate_0_4_2(config_obj)
	migrate_0_4_6(config_obj)
	migrate_0_5_6(config_obj)
}
