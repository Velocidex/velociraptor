// +build !aix

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package main

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/Velocidex/survey"
	"github.com/Velocidex/yaml/v2"
	jsonpatch "github.com/evanphx/json-patch"
	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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
		"api_client", "Dump an api_client config file.")

	config_api_client_common_name = config_api_client_command.Flag(
		"name", "The common name of the API Client.").
		Required().String()

	config_api_add_roles = config_api_client_command.Flag(
		"role", "Specify the role for this api_client.").
		String()

	config_api_client_password_protect = config_api_client_command.Flag(
		"password", "Protect the certificate with a password.").
		Bool()

	config_api_client_output = config_api_client_command.Arg(
		"output", "The filename to write the config file on.").
		Required().String()

	config_generate_command = config_command.Command(
		"generate",
		"Generate a new config file to stdout (with new keys).")

	config_generate_command_interactive = config_generate_command.Flag(
		"interactive", "Interactively fill in configuration.").
		Short('i').Bool()

	config_generate_command_merge = config_generate_command.Flag(
		"merge", "Merge this json config into the generated config").
		Strings()

	config_rotate_server_key = config_command.Command(
		"rotate_key",
		"Generate a new config file with a rotates server key.")
)

func verify_frontend_config(config_obj *config_proto.Config) error {
	server_cert, err := crypto.ParseX509CertFromPemStr(
		[]byte(config_obj.Frontend.Certificate))
	if err != nil {
		return err
	}

	if server_cert.PublicKeyAlgorithm != x509.RSA {
		return errors.New("Not RSA algorithm")
	}

	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))
	if !ok {
		return errors.New("failed to parse CA certificate")
	}

	// Verify that the certificate is signed by the CA.
	opts := x509.VerifyOptions{
		Roots: roots,
	}
	_, err = server_cert.Verify(opts)
	return err
}

func doShowConfig() {
	config_obj, err := DefaultConfigLoader.LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config.")

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}

	fmt.Printf("%v", string(res))
}

func generateNewKeys(config_obj *config_proto.Config) error {
	ca_bundle, err := crypto.GenerateCACert(2048)
	if err != nil {
		return errors.Wrap(err, "Unable to create CA cert")
	}

	config_obj.Client.CaCertificate = ca_bundle.Cert
	config_obj.CA.PrivateKey = ca_bundle.PrivateKey

	nonce := make([]byte, 8)
	_, err = rand.Read(nonce)
	if err != nil {
		return errors.Wrap(err, "Unable to create nonce")
	}
	config_obj.Client.Nonce = base64.StdEncoding.EncodeToString(nonce)

	// Make another nonce for VQL obfuscation.
	_, err = rand.Read(nonce)
	if err != nil {
		return errors.Wrap(err, "Unable to create nonce")
	}
	config_obj.ObfuscationNonce = base64.StdEncoding.EncodeToString(nonce)

	// Generate frontend certificate. Frontend certificates must
	// have a constant common name - clients will refuse to talk
	// with another common name.
	frontend_cert, err := crypto.GenerateServerCert(
		config_obj, config_obj.Client.PinnedServerName)
	if err != nil {
		return errors.Wrap(err, "Unable to create Frontend cert")
	}

	config_obj.Frontend.Certificate = frontend_cert.Cert
	config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Generate gRPC gateway certificate.
	gw_certificate, err := crypto.GenerateServerCert(
		config_obj, config_obj.API.PinnedGwName)
	if err != nil {
		return errors.Wrap(err, "Unable to create Frontend cert")
	}

	config_obj.GUI.GwCertificate = gw_certificate.Cert
	config_obj.GUI.GwPrivateKey = gw_certificate.PrivateKey

	return nil
}

func doGenerateConfigNonInteractive() {
	config_obj := config.GetDefaultConfig()

	err := generateNewKeys(config_obj)

	// Users have to updated the following fields.
	config_obj.Client.ServerUrls = []string{"https://localhost:8000/"}

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)
	if err != nil {
		logger.Error("Unable to create config", err)
		return
	}

	for _, merge_patch := range *config_generate_command_merge {
		serialized, err := json.Marshal(config_obj)
		if err != nil {
			logger.Error("Marshal config_obj")
			return
		}

		patched, err := jsonpatch.MergePatch(
			serialized, []byte(merge_patch))
		if err != nil {
			logger.Error("Invalid merge patch:", err)
			return
		}

		err = json.Unmarshal(patched, &config_obj)
		if err != nil {
			logger.Error("Patched object produces an invalid config: ", err)
			return
		}
	}

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		logger.Error("Unable to create config", err)
		return
	}
	fmt.Printf("%v", string(res))
}

func doRotateKeyConfig() {
	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config.")

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)

	// Frontends must have a well known common name.
	frontend_cert, err := crypto.GenerateServerCert(
		config_obj, config_obj.Client.PinnedServerName)
	if err != nil {
		logger.Error("Unable to create Frontend cert", err)
		return
	}

	config_obj.Frontend.Certificate = frontend_cert.Cert
	config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Generate gRPC gateway certificate.
	gw_certificate, err := crypto.GenerateServerCert(
		config_obj, config_obj.API.PinnedGwName)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to create gatewat cert")
	}

	config_obj.GUI.GwCertificate = gw_certificate.Cert
	config_obj.GUI.GwPrivateKey = gw_certificate.PrivateKey

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}
	fmt.Printf("%v", string(res))
}

func getClientConfig(config_obj *config_proto.Config) *config_proto.Config {
	// Copy only settings relevant to the client from the main
	// config.
	client_config := &config_proto.Config{
		Version: config_obj.Version,
		Client:  config_obj.Client,
	}

	return client_config
}

func doDumpClientConfig() {
	config_obj, err := DefaultConfigLoader.WithRequiredClient().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config.")

	client_config := getClientConfig(config_obj)
	res, err := yaml.Marshal(client_config)
	if err != nil {
		kingpin.FatalIfError(err, "Unable to encode config.")
	}
	fmt.Printf("%v", string(res))
}

func doDumpApiClientConfig() {
	config_obj, err := DefaultConfigLoader.WithRequiredCA().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config.")

	if *config_api_client_common_name == config_obj.Client.PinnedServerName {
		kingpin.Fatalf("Name reserved! You may not name your " +
			"api keys with this name.")
	}

	bundle, err := crypto.GenerateServerCert(
		config_obj, *config_api_client_common_name)
	kingpin.FatalIfError(err, "Unable to generate certificate.")

	if *config_api_client_password_protect {
		password := ""
		err = survey.AskOne(
			&survey.Password{Message: "Password:"},
			&password,
			survey.WithValidator(survey.Required))
		kingpin.FatalIfError(err, "Password.")

		pem_block, _ := pem.Decode([]byte(bundle.PrivateKey))
		if pem_block == nil {
			kingpin.Fatalf("Unable to decode private key.")
		}

		block, err := x509.EncryptPEMBlock(
			rand.Reader, "RSA PRIVATE KEY", pem_block.Bytes,
			[]byte(password), x509.PEMCipherAES256)
		kingpin.FatalIfError(err, "Password.")

		bundle.PrivateKey = string(pem.EncodeToMemory(block))
	}

	api_client_config := &config_proto.ApiClientConfig{
		CaCertificate:    config_obj.Client.CaCertificate,
		ClientCert:       bundle.Cert,
		ClientPrivateKey: string(bundle.PrivateKey),
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

	fd, err := os.OpenFile(*config_api_client_output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	kingpin.FatalIfError(err, "Unable to open output file: ")

	fd.Write(res)
	fd.Close()

	fmt.Printf("Creating API client file on %v.\n", *config_api_client_output)
	if *config_api_add_roles != "" {
		err = acls.GrantRoles(config_obj, *config_api_client_common_name,
			strings.Split(*config_api_add_roles, ","))
		kingpin.FatalIfError(err, "Unable to set role ACL: ")
	} else {
		fmt.Printf("No role added to user %v. You will need to do this later using the 'acl grant' command.", *config_api_client_common_name)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case config_show_command.FullCommand():
			doShowConfig()

		case config_generate_command.FullCommand():
			if *config_generate_command_interactive {
				doGenerateConfigInteractive()
			} else {
				doGenerateConfigNonInteractive()
			}

		case config_rotate_server_key.FullCommand():
			doRotateKeyConfig()

		case config_client_command.FullCommand():
			doDumpClientConfig()

		case config_api_client_command.FullCommand():
			doDumpApiClientConfig()

		case config_frontend_command.FullCommand():
			doConfigFrontend()

		default:
			return false
		}

		return true
	})
}
