/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/Velocidex/yaml/v2"
	errors "github.com/go-errors/errors"
	"software.sslmate.com/src/go-pkcs12"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/startup"
	vsurvey "www.velocidex.com/golang/velociraptor/tools/survey"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	config_command = app.Command(
		"config", "Manipulate the configuration.")

	config_command_org = config_command.Flag(
		"org", "Org ID to show").String()

	config_show_command = config_command.Command(
		"show", "Show the current config.")

	config_show_command_json = config_show_command.Flag(
		"json", "Show the config as JSON").Bool()

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

	config_api_client_pkcs12_output = config_api_client_command.Flag(
		"pkcs12", "A filename to write the pkcs12 certificate file").String()

	config_generate_command = config_command.Command(
		"generate",
		"Generate a new config file to stdout (with new keys).")

	config_generate_command_interactive = config_generate_command.Flag(
		"interactive", "Interactively fill in configuration.").
		Short('i').Bool()

	config_generate_command_merge = config_generate_command.Flag(
		"merge", "Merge this json config into the generated config (see https://datatracker.ietf.org/doc/html/rfc7396)").
		Strings()

	config_generate_command_merge_file = config_generate_command.Flag(
		"merge_file", "Merge this file containing a json config into the generated config (see https://datatracker.ietf.org/doc/html/rfc7396)").
		File()

	config_generate_command_patch = config_generate_command.Flag(
		"patch", "Patch this into the generated config (see http://jsonpatch.com/)").
		Strings()

	config_generate_command_patch_file = config_generate_command.Flag(
		"patch_file", "Patch this file into the generated config (see http://jsonpatch.com/)").
		File()

	config_rotate_server_key = config_command.Command(
		"rotate_keys",
		"Regenerate server private keys and reissue certificates.")

	config_rotate_server_key_validity = config_rotate_server_key.Flag(
		"validity",
		"How long should the new certs be valid for in days (default 365).").Int64()

	config_reissue_server_key = config_command.Command(
		"reissue_certs",
		"Reissue server certificates for the existing private keys.")

	config_reissue_server_key_validity = config_reissue_server_key.Flag(
		"validity",
		"How long should the new certs be valid for in days (default 365).").Int64()

	config_frontend_command = config_command.Command(
		"frontend", "Experimental: Create multi-frontend configuration")
)

func maybeGetOrgConfig(
	org_id string, config_obj *config_proto.Config) (
	*config_proto.Config, error) {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return config_obj, err
	}

	if org_id == "" {
		return config_obj, nil
	}

	config_obj, err = org_manager.GetOrgConfig(org_id)
	return config_obj, err
}

func doShowConfig() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		LoadAndValidate()
	if err != nil {
		return err
	}

	config_obj.Services = services.GenericToolServices()

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	config_obj, err = maybeGetOrgConfig(*config_command_org, config_obj)
	if err != nil {
		return err
	}

	err = applyMergesAndPatches(config_obj,
		*config_show_command_merge_file,
		*config_show_command_merge,
		*config_show_command_patch_file,
		*config_show_command_patch)
	if err != nil {
		return err
	}

	// Hide these fields which are autogenerated
	config_obj.Services = nil

	if *config_show_command_json {
		serialized, err := json.Marshal(config_obj)
		if err != nil {
			return err
		}
		fmt.Printf("%v", string(serialized))
		return nil
	}

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		return err
	}
	fmt.Printf("%v", string(res))

	return nil
}

func doGenerateConfigNonInteractive() error {
	logging.DisableLogging()

	// We have to suppress writing to stdout so users can redirect
	// output to a file.
	logging.SuppressLogging = true
	config_obj := config.GetDefaultConfig()

	err := vsurvey.GenerateNewKeys(config_obj)
	if err != nil {
		return fmt.Errorf("Unable to create config: %w", err)
	}

	// Users have to update the following fields.
	config_obj.Client.ServerUrls = []string{"https://localhost:8000/"}

	err = applyMergesAndPatches(config_obj,
		*config_generate_command_merge_file,
		*config_generate_command_merge,
		*config_generate_command_patch_file,
		*config_generate_command_patch)
	if err != nil {
		return err
	}
	res, err := yaml.Marshal(config_obj)
	if err != nil {
		return fmt.Errorf("Unable to create config: %w", err)
	}
	fmt.Printf("%v", string(res))
	return nil
}

func doRotateKeyConfig() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return err
	}

	if *config_rotate_server_key_validity > 0 {
		if config_obj.Security == nil {
			config_obj.Security = &config_proto.Security{}
		}
		config_obj.Security.CertificateValidityDays = *config_rotate_server_key_validity
	}

	// Frontends must have a well known common name.
	frontend_cert, err := crypto.GenerateServerCert(
		config_obj, utils.GetSuperuserName(config_obj))
	if err != nil {
		return fmt.Errorf("Unable to create Frontend cert: %w", err)
	}

	config_obj.Frontend.Certificate = frontend_cert.Cert
	config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Generate gRPC gateway certificate.
	gw_certificate, err := crypto.GenerateServerCert(
		config_obj, utils.GetGatewayName(config_obj))
	if err != nil {
		return err
	}

	config_obj.GUI.GwCertificate = gw_certificate.Cert
	config_obj.GUI.GwPrivateKey = gw_certificate.PrivateKey

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		return err
	}
	fmt.Printf("%v", string(res))

	return nil
}

func doReissueServerKeys() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return err
	}

	if *config_reissue_server_key_validity > 0 {
		if config_obj.Defaults == nil {
			config_obj.Defaults = &config_proto.Defaults{}
		}
		config_obj.Defaults.CertificateValidityDays = *config_reissue_server_key_validity
	}

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)

	frontend_cert, err := crypto.ReissueServerCert(
		config_obj, config_obj.Frontend.Certificate,
		config_obj.Frontend.PrivateKey)
	if err != nil {
		logger.Error("Unable to create Frontend cert: %v", err)
		return err
	}

	config_obj.Frontend.Certificate = frontend_cert.Cert
	config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Generate gRPC gateway certificate.
	gw_certificate, err := crypto.ReissueServerCert(
		config_obj, config_obj.GUI.GwCertificate,
		config_obj.GUI.GwPrivateKey)
	if err != nil {
		return fmt.Errorf("Unable to create gatewat cert: %w", err)
	}

	config_obj.GUI.GwCertificate = gw_certificate.Cert
	config_obj.GUI.GwPrivateKey = gw_certificate.PrivateKey

	res, err := yaml.Marshal(config_obj)
	if err != nil {
		return fmt.Errorf("Unable to encode config: %w", err)
	}
	fmt.Printf("%v", string(res))
	return nil
}

func doDumpClientConfig() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredClient().LoadAndValidate()
	if err != nil {
		return err
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	config_obj, err = maybeGetOrgConfig(*config_command_org, config_obj)
	if err != nil {
		return err
	}

	client_config := config.GetClientConfig(config_obj)
	res, err := yaml.Marshal(client_config)
	if err != nil {
		return fmt.Errorf("Unable to encode config: %w", err)
	}

	fmt.Printf("%v", string(res))
	return nil
}

func doDumpApiClientConfig() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredCA().
		WithRequiredUser().
		LoadAndValidate()
	if err != nil {
		return err
	}

	err = users.ValidateUsername(config_obj, *config_api_client_common_name)
	if err != nil {
		return err
	}

	if config_obj.Client == nil {
		return errors.New("Config does not have a client config!")
	}

	config_obj.Services = services.GenericToolServices()

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	bundle, err := crypto.GenerateServerCert(
		config_obj, *config_api_client_common_name)
	if err != nil {
		return fmt.Errorf("Unable to generate certificate: %w", err)
	}

	password := ""
	if *config_api_client_password_protect {
		password, err = vsurvey.GetAPIClientPassword()
		if err != nil {
			return err
		}

		pem_block, _ := pem.Decode([]byte(bundle.PrivateKey))
		if pem_block == nil {
			return fmt.Errorf("Unable to decode private key")
		}

		block, err := x509.EncryptPEMBlock(
			rand.Reader, "RSA PRIVATE KEY", pem_block.Bytes,
			[]byte(password), x509.PEMCipherAES256)
		if err != nil {
			return fmt.Errorf("Password: %w", err)
		}

		bundle.PrivateKey = string(pem.EncodeToMemory(block))
	}

	// Possibly dump out the pkcs12 key
	if *config_api_client_pkcs12_output != "" {
		ca_cert, err := crypto_utils.ParseX509CertFromPemStr([]byte(
			config_obj.Client.CaCertificate))
		if err != nil {
			return err
		}

		data, err := pkcs12.Encode(rand.Reader, bundle.PrivateKeyObj,
			bundle.Certificate, []*x509.Certificate{ca_cert}, password)
		if err != nil {
			return err
		}

		fd, err := os.OpenFile(*config_api_client_pkcs12_output,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		_, err = fd.Write(data)
		if err != nil {
			return err
		}
		fd.Close()

		fmt.Printf("Wrote PKCS12 file on %v.\n", *config_api_client_pkcs12_output)
	}

	api_client_config := &config_proto.ApiClientConfig{
		CaCertificate:    config_obj.Client.CaCertificate,
		ClientCert:       bundle.Cert,
		ClientPrivateKey: string(bundle.PrivateKey),
		Name:             *config_api_client_common_name,
	}

	switch config_obj.API.BindScheme {
	case "tcp":
		hostname := config_obj.API.Hostname
		if hostname == "" {
			hostname = config_obj.API.BindAddress
		}
		api_client_config.ApiConnectionString = fmt.Sprintf("%s:%v",
			hostname, config_obj.API.BindPort)
	case "unix":
		api_client_config.ApiConnectionString = fmt.Sprintf("unix://%s",
			config_obj.API.BindAddress)
	default:
		return fmt.Errorf("Unknown value for API.BindAddress")
	}

	res, err := yaml.Marshal(api_client_config)
	if err != nil {
		return fmt.Errorf("Unable to encode config: %w", err)
	}

	fd, err := os.OpenFile(*config_api_client_output,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("Unable to open output file: %w", err)
	}

	_, err = fd.Write(res)
	if err != nil {
		return fmt.Errorf("Unable to write output file: %w", err)
	}
	fd.Close()

	fmt.Printf("Creating API client file on %v.\n", *config_api_client_output)
	if *config_api_add_roles != "" {
		err = services.GrantRoles(config_obj, *config_api_client_common_name,
			strings.Split(*config_api_add_roles, ","))
		if err != nil {
			return fmt.Errorf("Unable to set role ACL: %w", err)
		}

		// Make sure the user actually exists.
		user_manager := services.GetUserManager()
		_, err = user_manager.GetUser(ctx,
			utils.GetSuperuserName(config_obj), *config_api_client_common_name)
		if err != nil {
			// Need to ensure we have a user
			err := user_manager.SetUser(ctx, &api_proto.VelociraptorUser{
				Name: *config_api_client_common_name,
			})
			if err != nil {
				return err
			}
		}

	} else {
		fmt.Printf("No role added to user %v. You will need to do this later using the 'acl grant' command.", *config_api_client_common_name)
	}
	return nil
}

func doGenerateConfigInteractive() error {
	config_obj, err := vsurvey.GetInteractiveConfig()
	if err != nil {
		return err
	}

	return vsurvey.StoreServerConfig(config_obj)
}

func doConfigFrontend() error {
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return err
	}

	if config_obj.Frontend == nil {
		return errors.New("Must provide a frontend config")
	}

	return vsurvey.GenerateFrontendPackages(config_obj)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case config_show_command.FullCommand():
			FatalIfError(config_show_command, doShowConfig)

		case config_generate_command.FullCommand():
			if *config_generate_command_interactive {
				FatalIfError(config_generate_command, doGenerateConfigInteractive)
			} else {
				FatalIfError(config_generate_command, doGenerateConfigNonInteractive)
			}

		case config_rotate_server_key.FullCommand():
			FatalIfError(config_rotate_server_key, doRotateKeyConfig)

		case config_reissue_server_key.FullCommand():
			FatalIfError(config_reissue_server_key, doReissueServerKeys)

		case config_client_command.FullCommand():
			FatalIfError(config_client_command, doDumpClientConfig)

		case config_api_client_command.FullCommand():
			FatalIfError(config_api_client_command, doDumpApiClientConfig)

		case config_frontend_command.FullCommand():
			FatalIfError(config_frontend_command, doConfigFrontend)

		default:
			return false
		}

		return true
	})
}
