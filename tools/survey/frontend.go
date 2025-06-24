package survey

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func reportCurrentSetup(config_obj *config_proto.Config) string {
	result := fmt.Sprintf("Master frontend is at %v:%v \n",
		config_obj.Frontend.Hostname, config_obj.Frontend.BindPort)

	if len(config_obj.ExtraFrontends) > 0 {
		result += fmt.Sprintf("Currently configured %v minion frontends\n\n",
			len(config_obj.ExtraFrontends))
		for idx, frontend := range config_obj.ExtraFrontends {
			result += fmt.Sprintf("Minion %v:  %v:%v\n", idx+1,
				frontend.Hostname, frontend.BindPort)
		}
	}

	return result
}

func GenerateFrontendPackages(config_obj *config_proto.Config) error {
	config := &ConfigSurvey{
		MinionHostname: config_obj.Frontend.Hostname,
		MinionBindPort: fmt.Sprintf("%v", config_obj.Frontend.BindPort),
		DynDNSType:     "none",
		UseWebsocket:   true,
	}

	if config_obj.Frontend.DynDns != nil {
		config.DynDNSType = config_obj.Frontend.DynDns.Type
		config.DdnsUsername = config_obj.Frontend.DynDns.DdnsUsername
		config.DdnsPassword = config_obj.Frontend.DynDns.DdnsPassword
		config.ZoneName = config_obj.Frontend.DynDns.ZoneName
		config.ApiToken = config_obj.Frontend.DynDns.ApiToken
	}

	items := []huh.Field{
		huh.NewNote().
			Title("Welcome to the Velociraptor multi-frontend configuration generator").
			Description(`Warning: This configuration is currently experimental. Read more about
it here https://docs.velociraptor.app/docs/deployment/cloud/multifrontend/

I will be adding an extra minion frontend to the configuration.

You can use the new configuration to create master and minion debian packages using:

velociraptor --config server.config.yaml debian server

` + reportCurrentSetup(config_obj)),
		huh.NewInput().
			Title("What is the public name of the minion frontend?").
			Description("A public DNS name is how clients can reach the minion  (e.g. 192.168.1.22 or ns2.example.com).").
			Value(&config.MinionHostname),
		huh.NewInput().
			Title("What port should the minion listen on?").
			Validate(validate_int("Minion Port Number")).
			Value(&config.MinionBindPort),
	}

	for {
		form := huh.NewForm(huh.NewGroup(items...)).WithTheme(getTheme())
		err := form.Run()
		if err != nil {
			return err
		}

		questions := configureDynDNS(config)
		if questions != nil {
			form := huh.NewForm(huh.NewGroup(questions...)).WithTheme(getTheme())
			err := form.Run()
			if err != nil {
				return err
			}
		}

		new_config, err := config.compileFrontend(config_obj)
		if err != nil {
			if showError("Error building new configuration",
				"Retry to create again, or abort", err) {
				continue
			}
			return err
		}

		err = config.checkFrontendWithUser(new_config)
		if err != nil {
			return err
		}

		return StoreServerConfig(new_config)
	}
}

func (self *ConfigSurvey) checkFrontendWithUser(
	config_obj *config_proto.Config) error {
	if len(config_obj.ExtraFrontends) == 0 {
		return errors.New("No new minions configured")
	}

	last_fe := config_obj.ExtraFrontends[len(config_obj.ExtraFrontends)-1]
	message := fmt.Sprintf(`
Added a new minion with node %v.

Clients will connect to it using the URL %v.

The master API bind addres was changed to 0.0.0.0 to allow minion connections.

Master datastore implementation was set to %v
Minion datastore implementation was set to %v

NOTE: Both Master and Minion expect an EFS volume mounted on %v within their respective VM/container. Please ensure this is true in deployment!

`, services.GetNodeName(last_fe), self.getURL(last_fe),
		config_obj.Datastore.MasterImplementation,
		config_obj.Datastore.MinionImplementation,
		config_obj.Datastore.Location,
	)

	message += "Clients will load balance between the following frontends:\n"
	for _, url := range config_obj.Client.ServerUrls {
		message += fmt.Sprintf("  %v\n", url)
	}

	goahead := true

	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Configuration overview").
			Description(message).
			Value(&goahead))).WithTheme(getTheme())
	err := form.Run()
	if err != nil {
		return err
	}

	if !goahead {
		return errors.New("Cancelled by user")
	}

	return nil
}

func (self *ConfigSurvey) compileFrontend(
	config_obj *config_proto.Config) (*config_proto.Config, error) {
	new_config := proto.Clone(config_obj).(*config_proto.Config)

	frontend_config := &config_proto.FrontendConfig{
		BindAddress: "0.0.0.0",
	}

	frontend_config.BindAddress = "0.0.0.0"
	frontend_config.Hostname = self.MinionHostname
	port, _ := utils.ToInt64(self.MinionBindPort)
	frontend_config.BindPort = uint32(port)

	// Check for validity
	node_name := services.GetNodeName(frontend_config)
	if node_name == services.GetNodeName(config_obj.Frontend) {
		return nil, fmt.Errorf(
			"Minion node name %v is the same as existing master %v",
			node_name, services.GetNodeName(config_obj.Frontend))
	}

	for _, fe := range config_obj.ExtraFrontends {
		if node_name == services.GetNodeName(fe) {
			return nil, fmt.Errorf(
				"Minion node name %v is the same as existing minion %v",
				node_name, services.GetNodeName(fe))
		}
	}

	// Add the additional frontend.
	new_config.ExtraFrontends = append(new_config.ExtraFrontends,
		frontend_config)

	// API server must be exposed to allow multiple frontends to call
	// it.
	new_config.API.BindAddress = "0.0.0.0"

	// Enable caching datastores
	new_config.Datastore.MasterImplementation = "MemcacheFileDataStore"
	new_config.Datastore.MinionImplementation = "RemoteFileDataStore"
	new_config.Datastore.Implementation = ""

	// Adjust clients' config to load balance to all frontends.
	for _, fe := range new_config.ExtraFrontends {
		connection_string := self.getURL(fe)

		// Add the frontend into the client's configuration.
		if !utils.InString(new_config.Client.ServerUrls, connection_string) {
			new_config.Client.ServerUrls = append(
				new_config.Client.ServerUrls, connection_string)
		}
	}

	return new_config, nil
}

func (self *ConfigSurvey) getURL(fe *config_proto.FrontendConfig) string {
	protocol := "https"
	if self.UseWebsocket {
		protocol = "wss"
	}

	if fe.BindPort != 443 {
		return fmt.Sprintf(
			"%v://%v:%v/", protocol, fe.Hostname, fe.BindPort)
	}
	return fmt.Sprintf("%v://%v/", protocol, fe.Hostname)
}
