package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"

	"github.com/Velocidex/yaml"
	"gopkg.in/alecthomas/kingpin.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	config_command = app.Command(
		"config", "Manipulate the configuration.")
	config_show_command = config_command.Command(
		"show", "Show the current config.")
	config_client_command = config_command.Command(
		"client", "Dump the client's config file.")

	config_api_client_command = config_command.Command(
		"api_client", "Dump and api_client config file.")

	config_api_client_common_name = config_api_client_command.Flag(
		"name", "The common name of the API Client.").
		Default("Velociraptor API Client").String()

	config_generate_command = config_command.Command(
		"generate",
		"Generate a new config file to stdout (with new keys).")
	config_rotate_server_key = config_command.Command(
		"rotate_key",
		"Generate a new config file with a rotates server key.")
)

func doShowConfig() {
	config_obj, err := config.LoadClientConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")

	// Dump out the embedded config as is.
	if *config_path == "" {
		content := string(config.FileConfigDefaultYaml)
		content = regexp.MustCompile(`##[^\n]+\n`).ReplaceAllString(content, "")
		fmt.Printf("%v", content)
		return
	}

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}

	fmt.Printf("%v", string(res))
}

func doGenerateConfig() {
	config_obj := config.GetDefaultConfig()
	logger := logging.GetLogger(config_obj, &logging.ToolComponent)
	ca_bundle, err := crypto.GenerateCACert(2048)
	if err != nil {
		logger.Error("Unable to create CA cert", err)
		return
	}

	config_obj.Client.CaCertificate = ca_bundle.Cert
	config_obj.CA.PrivateKey = ca_bundle.PrivateKey

	nonce := make([]byte, 8)
	_, err = rand.Read(nonce)
	if err != nil {
		logger.Error("Unable to create nonce", err)
		return
	}
	config_obj.Client.Nonce = base64.StdEncoding.EncodeToString(nonce)

	// Generate frontend certificate.
	frontend_cert, err := crypto.GenerateServerCert(config_obj, constants.FRONTEND_NAME)
	if err != nil {
		logger.Error("Unable to create Frontend cert", err)
		return
	}

	config_obj.Frontend.Certificate = frontend_cert.Cert
	config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Users have to updated the following fields.
	config_obj.Client.ServerUrls = []string{"http://localhost:8000/"}

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		logger.Error("Unable to create CA cert", err)
		return
	}
	fmt.Printf("%v", string(res))
}

func doRotateKeyConfig() {
	config_obj, err := config.LoadConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")
	logger := logging.GetLogger(config_obj, &logging.ToolComponent)
	frontend_cert, err := crypto.GenerateServerCert(config_obj, constants.FRONTEND_NAME)
	if err != nil {
		logger.Error("Unable to create Frontend cert", err)
		return
	}

	config_obj.Frontend.Certificate = frontend_cert.Cert
	config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}
	fmt.Printf("%v", string(res))
}

func doDumpClientConfig() {
	config_obj, err := config.LoadConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")

	// Copy only settings relevant to the client from the main
	// config.
	client_config := &api_proto.Config{
		Version: config_obj.Version,
		Client:  config_obj.Client,

		// Clients will change their SSL requirements for self
		// signing.
		UseSelfSignedSsl: config_obj.UseSelfSignedSsl,
	}

	res, err := yaml.Marshal(client_config)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}
	fmt.Printf("%v", string(res))
}

func doDumpApiClientConfig() {
	config_obj, err := config.LoadConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")

	bundle, err := crypto.GenerateServerCert(config_obj, *config_api_client_common_name)
	kingpin.FatalIfError(err, "Unable to generate certificate.")

	api_client_config := &api_proto.ApiClientConfig{
		CaCertificate:    config_obj.Client.CaCertificate,
		ClientCert:       bundle.Cert,
		ClientPrivateKey: bundle.PrivateKey,
		Name:             *config_api_client_common_name,
	}

	switch config_obj.API.BindScheme {
	case "tcp":
		api_client_config.ApiConnectionString = fmt.Sprintf("%s:%v",
			config_obj.API.BindAddress, config_obj.API.BindPort)
	case "unix":
		api_client_config.ApiConnectionString = fmt.Sprintf("unix://%s",
			config_obj.API.BindAddress)
	default:
		kingpin.Fatalf("Unknown value for API.BindAddress")
	}

	res, err := yaml.Marshal(api_client_config)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}
	fmt.Printf("%v", string(res))
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case config_show_command.FullCommand():
			doShowConfig()

		case config_generate_command.FullCommand():
			doGenerateConfig()

		case config_rotate_server_key.FullCommand():
			doRotateKeyConfig()

		case config_client_command.FullCommand():
			doDumpClientConfig()

		case config_api_client_command.FullCommand():
			doDumpApiClientConfig()

		default:
			return false
		}

		return true
	})
}
