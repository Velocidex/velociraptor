package main

import (
	"fmt"
	"gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func doShowConfig() {
	config_obj, err := get_config(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")

	res, err := config.Encode(config_obj)
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

	frontend_cert, err := crypto.GenerateServerCert(config_obj)
	if err != nil {
		logger.Error("Unable to create Frontend cert", err)
		return
	}

	config_obj.Frontend.Certificate = frontend_cert.Cert
	config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Users have to updated the following fields.
	config_obj.Client.ServerUrls = []string{"http://localhost:8000/"}

	res, err := config.Encode(config_obj)
	if err != nil {
		logger.Error("Unable to create CA cert", err)
		return
	}
	fmt.Printf("%v", string(res))
}

func doRotateKeyConfig() {
	config_obj, err := get_config(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")
	logger := logging.NewLogger(config_obj)
	frontend_cert, err := crypto.GenerateServerCert(config_obj)
	if err != nil {
		logger.Error("Unable to create Frontend cert", err)
		return
	}

	config_obj.Frontend.Certificate = frontend_cert.Cert
	config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	res, err := config.Encode(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}
	fmt.Printf("%v", string(res))
}

func doDumpClientConfig() {
	config_obj, err := get_config(*config_path)
	kingpin.FatalIfError(err, "Unable to load config.")

	client_config := config.NewClientConfig()
	client_config.Client = config_obj.Client

	res, err := config.Encode(client_config)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}
	fmt.Printf("%v", string(res))
}
