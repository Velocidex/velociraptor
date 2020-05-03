// +build !aix

package main

import (
	"fmt"
	"os"

	"github.com/Velocidex/survey"
	"github.com/Velocidex/yaml/v2"
	proto "github.com/golang/protobuf/proto"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
)

var (
	config_frontend_command = config_command.Command(
		"frontend", "Add a new frontend configuration")
)

func doConfigFrontend() {
	config_obj, err := config.LoadConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")
	kingpin.FatalIfError(config.ValidateFrontendConfig(config_obj),
		"Unable to load config.")
	if config_obj.Datastore.Implementation == "FileBaseDataStore" {
		kingpin.Fatalf("Current FileStore implementation is %v which does not support multiple frontends.", config_obj.Datastore.Implementation)
	}

	frontend_config := &config_proto.FrontendConfig{}
	proto.Merge(frontend_config, config_obj.Frontend)

	// Set a better default for the url question
	url_question.Default = config_obj.Frontend.Hostname

	// Figure out the install type
	if config_obj.Client.UseSelfSignedSsl {
		kingpin.FatalIfError(survey.Ask([]*survey.Question{
			{Name: "Hostname", Prompt: url_question},
			{Name: "BindPort", Prompt: port_question},
		}, frontend_config, survey.WithValidator(survey.Required)), "")
	} else {
		kingpin.FatalIfError(survey.AskOne(url_question, &frontend_config.Hostname,
			survey.WithValidator(survey.Required)), "")
	}

	if config_obj.Frontend.DynDns.DdnsUsername != "" {
		kingpin.FatalIfError(dynDNSConfig(config_obj), "")
	}

	// Frontends must have a well known common name.
	fmt.Println("Generating keys please wait....")
	frontend_cert, err := crypto.GenerateServerCert(
		config_obj, config_obj.Client.PinnedServerName)
	kingpin.FatalIfError(err, "Unable to create Frontend cert")

	frontend_config.Certificate = frontend_cert.Cert
	frontend_config.PrivateKey = frontend_cert.PrivateKey

	// Add the additional frontend.
	config_obj.ExtraFrontends = append(config_obj.ExtraFrontends,
		frontend_config)

	// API server must be exposed to allow multiple frontends to
	// call it.
	config_obj.API.BindAddress = "0.0.0.0"

	path := ""
	err = survey.AskOne(output_question, &path,
		survey.WithValidator(survey.Required))
	kingpin.FatalIfError(err, "Question")

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}

	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	kingpin.FatalIfError(err, "Open file %s", path)
	_, err = fd.Write(res)
	kingpin.FatalIfError(err, "Write file %s", path)
	fd.Close()
}
