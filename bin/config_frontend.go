// +build !aix

package main

import (
	"fmt"

	"github.com/Velocidex/survey"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	config_frontend_command = config_command.Command(
		"frontend", "Add a new frontend configuration")
)

func reportCurrentSetup(config_obj *config_proto.Config) string {
	result := fmt.Sprintf("Master frontend is at %v:%v \n\n",
		config_obj.Frontend.Hostname, config_obj.Frontend.BindPort)

	if len(config_obj.ExtraFrontends) > 0 {
		result += fmt.Sprintf("Currently configured %v minion frontends\n\n", len(config_obj.ExtraFrontends))
		for idx, frontend := range config_obj.ExtraFrontends {
			result += fmt.Sprintf("Minion %v:  %v:%v\n", idx+1,
				frontend.Hostname, frontend.BindPort)
		}
	}

	return result + "\n"
}

func doConfigFrontend() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config.")

	doit := false
	kingpin.FatalIfError(survey.AskOne(&survey.Confirm{
		Message: `
Welcome to the Velociraptor configuration generator
---------------------------------------------------

I will be adding an extra minion frontend to the configuration. I will not be changing the master frontend configuration at all.

` + reportCurrentSetup(config_obj) + `

Do you wish to continue?`,
	},
		&doit, survey.WithValidator(survey.Required)), "")

	if !doit {
		return
	}

	// Create a new frontend config
	frontend_config := &config_proto.FrontendConfig{
		BindAddress: "0.0.0.0",
	}

	// Set a better default for the url question
	url_question.Default = config_obj.Frontend.Hostname

	// Figure out the install type
	if config_obj.Client.UseSelfSignedSsl {
		kingpin.FatalIfError(survey.Ask([]*survey.Question{
			{Name: "Hostname", Prompt: &survey.Input{
				Message: "What is the public name of the minion frontend (e.g. 192.168.1.22 or ns2.example.com)",
				Default: config_obj.Frontend.Hostname,
			}},
			{Name: "BindPort", Prompt: &survey.Input{
				Message: "What port should this frontend listen on?",
				Default: fmt.Sprintf("%v", config_obj.Frontend.BindPort),
			}},
		}, frontend_config, survey.WithValidator(survey.Required)), "")

		// Add the frontend into the client's configuration.
		config_obj.Client.ServerUrls = append(config_obj.Client.ServerUrls,
			fmt.Sprintf("https://%v:%v/", frontend_config.Hostname,
				frontend_config.BindPort))
	} else {
		kingpin.FatalIfError(survey.AskOne(url_question, &frontend_config.Hostname,
			survey.WithValidator(survey.Required)), "")
	}

	if config_obj.Frontend.DynDns != nil &&
		config_obj.Frontend.DynDns.DdnsUsername != "" {
		kingpin.FatalIfError(dynDNSConfig(config_obj.Frontend), "")
	}

	// Add the additional frontend.
	config_obj.ExtraFrontends = append(config_obj.ExtraFrontends,
		frontend_config)

	// API server must be exposed to allow multiple frontends to
	// call it.
	config_obj.API.BindAddress = "0.0.0.0"

	storeServerConfig(config_obj)
}
