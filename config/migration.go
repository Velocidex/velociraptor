package config

import (
	"regexp"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func deprecated(config_obj *config_proto.Config, name string) {
	logging.Prelog("Config contains deprecated field %v", name)
}

// Migrate from pre 0.4.2 config files.
func migrate_0_4_2(config_obj *config_proto.Config) {
	if config_obj.AutocertDomain != "" {
		deprecated(config_obj, "autocert_domain")
		config_obj.Frontend.Hostname = config_obj.AutocertDomain
		config_obj.AutocertDomain = ""
	}

	local_buffer := config_obj.Client.LocalBuffer
	if local_buffer != nil && local_buffer.Filename != "" {
		deprecated(config_obj, "Client.local_buffer.filename")
		local_buffer.Filename = ""
	}

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

}

func migrate(config_obj *config_proto.Config) {
	migrate_0_4_2(config_obj)
}
