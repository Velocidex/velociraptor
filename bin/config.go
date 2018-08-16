package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/ghodss/yaml"
	"gopkg.in/alecthomas/kingpin.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
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
	config_generate_command = config_command.Command(
		"generate",
		"Generate a new config file to stdout (with new keys).")
	config_rotate_server_key = config_command.Command(
		"rotate_key",
		"Generate a new config file with a rotates server key.")
)

func doShowConfig() {
	if *config_path == "" {
		kingpin.Fatalf("Config file must be specified.")
	}
	config_obj, err := config.LoadClientConfig(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}

	fmt.Printf("%v", string(res))
}

func doGenerateConfig() {
	config_obj := config.GetDefaultConfig()
	logger := logging.NewLogger(config_obj)
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
	frontend_cert, err := crypto.GenerateServerCert(config_obj)
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
	logger := logging.NewLogger(config_obj)
	frontend_cert, err := crypto.GenerateServerCert(config_obj)
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

	client_config := &api_proto.Config{
		Version: config_obj.Version,
		Client:  config_obj.Client,
	}

	res, err := yaml.Marshal(client_config)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}
	fmt.Printf("%v", string(res))
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "config show":
			doShowConfig()

		case "config generate":
			doGenerateConfig()

		case "config rotate_key":
			doRotateKeyConfig()

		case "config client":
			doDumpClientConfig()
		default:
			return false
		}

		return true
	})
}
