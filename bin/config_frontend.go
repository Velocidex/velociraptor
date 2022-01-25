// +build !aix

package main

import (
	"fmt"

	"github.com/Velocidex/survey"
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	config_frontend_command = config_command.Command(
		"frontend", "Experimental: Create multi-frontend configuration")
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

func doConfigFrontend() error {
	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return err
	}

	doit := false
	err = survey.AskOne(&survey.Confirm{
		Message: `
Welcome to the Velociraptor multi-frontend configuration generator
------------------------------------------------------------------

Warning: This configuration is currently experiemental. Read more about
it here https://docs.velociraptor.app/docs/deployment/cloud/multifrontend/

I will be adding an extra minion frontend to the configuration.
I will not be changing the master frontend configuration at all.

` + reportCurrentSetup(config_obj) + `

Do you wish to continue?`,
	}, &doit, survey.WithValidator(survey.Required))
	if err != nil {
		return err
	}
	if !doit {
		return err
	}

	// Create a new frontend config
	frontend_config := &config_proto.FrontendConfig{
		BindAddress: "0.0.0.0",
	}

	// Set a better default for the url question
	url_question.Default = config_obj.Frontend.Hostname

	// Figure out the install type
	if config_obj.Client.UseSelfSignedSsl {
		err = survey.Ask([]*survey.Question{
			{Name: "Hostname", Prompt: &survey.Input{
				Message: "What is the public name of the minion frontend (e.g. 192.168.1.22 or ns2.example.com)",
				Default: config_obj.Frontend.Hostname,
			}},
			{Name: "BindPort", Prompt: &survey.Input{
				Message: "What port should this frontend listen on?",
				Default: fmt.Sprintf("%v", config_obj.Frontend.BindPort),
			}},
		}, frontend_config, survey.WithValidator(survey.Required))
		if err != nil {
			return err
		}

		// Add the frontend into the client's configuration.
		if frontend_config.BindPort != 443 {
			config_obj.Client.ServerUrls = append(config_obj.Client.ServerUrls,
				fmt.Sprintf("https://%v:%v/", frontend_config.Hostname,
					frontend_config.BindPort))
		} else {
			config_obj.Client.ServerUrls = append(config_obj.Client.ServerUrls,
				fmt.Sprintf("https://%v/", frontend_config.Hostname))
		}

	} else {
		err = survey.AskOne(url_question, &frontend_config.Hostname,
			survey.WithValidator(survey.Required))
		if err != nil {
			return err
		}
		frontend_config.BindPort = 443
	}

	// Check for validity
	if services.GetNodeName(frontend_config) ==
		services.GetNodeName(config_obj.Frontend) {
		return errors.New("Node name is the same as existing master")
	}

	for _, fe := range config_obj.ExtraFrontends {
		if services.GetNodeName(frontend_config) ==
			services.GetNodeName(fe) {
			return errors.New("Node name is the same as an existing minion")
		}
	}

	if config_obj.Frontend.DynDns != nil &&
		config_obj.Frontend.DynDns.DdnsUsername != "" {
		err = dynDNSConfig(frontend_config)
		if err != nil {
			return err
		}
	}

	// Add the additional frontend.
	config_obj.ExtraFrontends = append(config_obj.ExtraFrontends,
		frontend_config)

	// API server must be exposed to allow multiple frontends to
	// call it.
	fmt.Println("I will enable API server to listen on all ports")
	config_obj.API.BindAddress = "0.0.0.0"

	// Enable caching datastores
	fmt.Println("Master will use MemcacheFileDataStore (Memory cached filestore)")
	config_obj.Datastore.MasterImplementation = "MemcacheFileDataStore"

	fmt.Println("Minion will use RemoteFileDataStore (replicating to master).")
	config_obj.Datastore.MinionImplementation = "RemoteFileDataStore"
	config_obj.Datastore.Implementation = ""

	fmt.Printf("\nNOTE: Both Master and Minion expect an EFS volume mounted on %v within their respective VM/container.\nPlease ensure this is true in deployment!\n\n",
		config_obj.Datastore.Location)

	// Adjust clients' config to load balance to all frontends.
	for _, fe := range config_obj.ExtraFrontends {
		connection_string := fmt.Sprintf("https://%v:%v/",
			fe.Hostname, fe.BindPort)

		if !utils.InString(config_obj.Client.ServerUrls, connection_string) {
			config_obj.Client.ServerUrls = append(config_obj.Client.ServerUrls,
				connection_string)
		}
	}

	fmt.Printf("Clients will load balance between the following frontends:\n")
	for _, url := range config_obj.Client.ServerUrls {
		fmt.Printf("  %v\n", url)
	}

	err = storeServerConfig(config_obj)
	if err != nil {
		return err
	}

	return storeClientConfig(config_obj)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case config_frontend_command.FullCommand():
			FatalIfError(config_frontend_command, doConfigFrontend)

		default:
			return false
		}

		return true
	})
}
