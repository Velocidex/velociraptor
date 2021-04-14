// +build !aix

package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"reflect"
	"regexp"
	"runtime"

	"github.com/Velocidex/survey"
	"github.com/Velocidex/yaml/v2"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/users"
)

const (
	self_signed = "Self Signed SSL"
	autocert    = "Automatically provision certificates with Lets Encrypt"
	oauth_sso   = "Authenticate users with SSO"

	// FileStore implementations
	// mysql_datastore     = "MySQL"
	filebased_datastore = "FileBaseDataStore"
)

var (
	sso_type = &survey.Select{
		Message: "Select the SSO Authentication Provider",
		Default: "Google",
		Options: []string{"Google", "GitHub", "Azure", "OIDC"},
	}

	server_type_question = &survey.Select{
		Message: `
Welcome to the Velociraptor configuration generator
---------------------------------------------------

I will be creating a new deployment configuration for you. I will
begin by identifying what type of deployment you need.


What OS will the server be deployed on?
`,
		Default: runtime.GOOS,
		Options: []string{"linux", "windows", "darwin"},
	}

	url_question = &survey.Input{
		Message: "What is the public DNS name of the Master Frontend " +
			"(e.g. www.example.com):",
		Help: "Clients will connect to the Frontend using this " +
			"public name (e.g. https://www.example.com:8000/ ).",
		Default: "localhost",
	}

	// https://docs.microsoft.com/en-us/troubleshoot/windows-server/identity/naming-conventions-for-computer-domain-site-ou#dns-host-names
	url_validator = regexValidator("^[a-z0-9.A-Z\\-]+$")
	port_question = &survey.Input{
		Message: "Enter the frontend port to listen on.",
		Default: "8000",
	}
	port_validator = regexValidator("^[0-9]+$")

	gui_port_question = &survey.Input{
		Message: "Enter the port for the GUI to listen on.",
		Default: "8889",
	}

	log_question = &survey.Input{
		Message: "Path to the logs directory.",
		Default: os.TempDir(),
	}

	output_question = &survey.Input{
		Message: "Where should i write the server config file?",
		Default: "server.config.yaml",
	}

	client_output_question = &survey.Input{
		Message: "Where should i write the client config file?",
		Default: "client.config.yaml",
	}

	user_name_question = &survey.Input{
		Message: "GUI Username or email address to authorize (empty to end):",
	}

	password_question = &survey.Password{
		Message: "Password",
	}

	google_oauth = []*survey.Question{
		{
			Name: "OauthClientId",
			Prompt: &survey.Input{
				Message: "Enter the OAuth Client ID?",
			},
		}, {
			Name: "OauthClientSecret",
			Prompt: &survey.Input{
				Message: "Enter the OAuth Client Secret?",
			},
		},
	}

	google_domains_username = &survey.Input{
		Message: "Google Domains DynDNS Username",
	}

	google_domains_password = &survey.Input{
		Message: "Google Domains DynDNS Password",
	}
)

func regexValidator(re string) survey.Validator {
	compiled_re := regexp.MustCompile(re)

	return func(val interface{}) error {
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("cannot regex on type %v", reflect.TypeOf(val).Name())
		}

		match := compiled_re.MatchString(s)
		if !match {
			return fmt.Errorf("Invalid format")
		}
		return nil
	}
}

func configureDataStore(config_obj *config_proto.Config) {
	// For now the file based datastore is the only one supported.
	config_obj.Datastore.Implementation = filebased_datastore

	// Configure the data store
	var default_data_store string
	switch config_obj.ServerType {
	case "windows":
		default_data_store = "C:\\Windows\\Temp"
	default:
		default_data_store = "/opt/velociraptor"
	}

	data_store_file := []*survey.Question{
		{
			Name: "Location",
			Prompt: &survey.Input{
				Message: "Path to the datastore directory.",
				Default: default_data_store,
			},
		},
	}

	kingpin.FatalIfError(
		survey.Ask(data_store_file,
			config_obj.Datastore,
			survey.WithValidator(survey.Required)), "")

	config_obj.Datastore.FilestoreDirectory = config_obj.Datastore.Location
	log_question.Default = path.Join(config_obj.Datastore.Location, "logs")
}

func configureDeploymentType(config_obj *config_proto.Config) {
	// What type of install do we need?
	install_type := ""
	kingpin.FatalIfError(survey.AskOne(&survey.Select{
		Options: []string{self_signed, autocert, oauth_sso},
	}, &install_type, nil), "")

	switch install_type {
	case self_signed:
		kingpin.FatalIfError(configSelfSigned(config_obj), "")

	case autocert:
		kingpin.FatalIfError(configAutocert(config_obj), "")

	case oauth_sso:
		kingpin.FatalIfError(configAutocert(config_obj), "")

		config_obj.AutocertCertCache = config_obj.Datastore.Location
		config_obj.GUI.Authenticator = &config_proto.Authenticator{}
		configureSSO(config_obj)
	}

}

func doGenerateConfigInteractive() {
	config_obj := config.GetDefaultConfig()

	// Figure out which type of server we have.
	kingpin.FatalIfError(
		survey.AskOne(server_type_question,
			&config_obj.ServerType,
			survey.WithValidator(survey.Required)), "")

	configureDataStore(config_obj)
	configureDeploymentType(config_obj)

	// The API's public DNS name allows external callers but by
	// default we bind to loopback only.
	config_obj.API.Hostname = config_obj.Frontend.Hostname
	config_obj.API.BindAddress = "127.0.0.1"

	// Setup dyndns
	kingpin.FatalIfError(dynDNSConfig(config_obj.Frontend), "")

	// Add users to the config file so the server can be
	// initialized.
	kingpin.FatalIfError(addUser(config_obj), "Add users")

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)
	logger.Info("Generating keys please wait....")
	kingpin.FatalIfError(generateNewKeys(config_obj), "")

	kingpin.FatalIfError(survey.AskOne(log_question,
		&config_obj.Logging.OutputDirectory,
		survey.WithValidator(survey.Required)), "")

	config_obj.Logging.SeparateLogsPerComponent = true

	// By default disabled debug logging - it is not useful unless
	// you are trying to debug something.
	config_obj.Logging.Debug = &config_proto.LoggingRetentionConfig{
		Disabled: true,
	}

	storeServerConfig(config_obj)
	storeClientConfig(config_obj)
}

func storeClientConfig(config_obj *config_proto.Config) {
	path := ""
	kingpin.FatalIfError(survey.AskOne(client_output_question, &path,
		survey.WithValidator(survey.Required)), "")

	client_config := getClientConfig(config_obj)
	res, err := yaml.Marshal(client_config)
	kingpin.FatalIfError(err, "Yaml Marshal")

	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	kingpin.FatalIfError(err, "Open file %s", path)
	_, err = fd.Write(res)
	kingpin.FatalIfError(err, "Write file %s", path)
	fd.Close()
}

func storeServerConfig(config_obj *config_proto.Config) {
	path := ""
	kingpin.FatalIfError(
		survey.AskOne(output_question, &path,
			survey.WithValidator(survey.Required)), "")

	res, err := yaml.Marshal(config_obj)
	kingpin.FatalIfError(err, "Yaml Marshal")

	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	kingpin.FatalIfError(err, "Open file %s", path)
	_, err = fd.Write(res)
	kingpin.FatalIfError(err, "Write file %s", path)
	fd.Close()
}

func configureSSO(config_obj *config_proto.Config) {
	// Which flavor of SSO do we want?
	kingpin.FatalIfError(
		survey.AskOne(sso_type,
			&config_obj.GUI.Authenticator.Type,
			survey.WithValidator(survey.Required)), "")

	// Provide the user with a hint about the redirect URL
	redirect := ""
	switch config_obj.GUI.Authenticator.Type {
	case "Google":
		redirect = config_obj.GUI.PublicUrl + "auth/google/callback"
	case "GitHub":
		redirect = config_obj.GUI.PublicUrl + "auth/github/callback"
	case "Azure":
		redirect = config_obj.GUI.PublicUrl + "auth/azure/callback"
	case "OIDC":
		redirect = config_obj.GUI.PublicUrl + "auth/oidc/callback"
	}
	fmt.Printf("\nSetting %v configuration will use redirect URL %v\n",
		config_obj.GUI.Authenticator.Type, redirect)

	switch config_obj.GUI.Authenticator.Type {
	case "Google", "GitHub":
		kingpin.FatalIfError(survey.Ask(google_oauth,
			config_obj.GUI.Authenticator,
			survey.WithValidator(survey.Required)), "")

	case "Azure":
		// Azure also requires the tenant ID
		google_oauth = append(google_oauth, &survey.Question{
			Name: "Tenant",
			Prompt: &survey.Input{
				Message: "Enter the Tenant Domain name or ID?",
			},
		})

		kingpin.FatalIfError(survey.Ask(google_oauth,
			config_obj.GUI.Authenticator,
			survey.WithValidator(survey.Required)), "")
	case "OIDC":
		// OIDC require Issuer URL
		google_oauth = append(google_oauth, &survey.Question{
			Name: "OidcIssuer",
			Prompt: &survey.Input{
				Message: "Enter valid OIDC Issuer URL",
				Help:    "e.g. https://accounts.google.com or https://your-org-name.okta.com are valid Issuer URLs, check that URL has /.well-known/openid-configuration endpoint",
			},
			Validate: func(val interface{}) error {
				// A check to avoid double slashes
				if str, ok := val.(string); !ok || str[len(str)-1:] == "/" {
					return fmt.Errorf("Issuer URL should not have / (slash) sign as the last symbol")
				}
				return nil
			},
		})

		kingpin.FatalIfError(survey.Ask(google_oauth,
			config_obj.GUI.Authenticator,
			survey.WithValidator(survey.Required)), "")
	}
}

func dynDNSConfig(frontend *config_proto.FrontendConfig) error {
	dyndns := false
	err := survey.AskOne(&survey.Confirm{
		Message: "Are you using Google Domains DynDNS?"},
		&dyndns, survey.WithValidator(survey.Required))
	if err != nil {
		return err
	}

	if !dyndns {
		return nil
	}

	return survey.Ask([]*survey.Question{
		{Name: "DdnsUsername", Prompt: google_domains_username},
		{Name: "DdnsPassword", Prompt: google_domains_password},
	}, frontend.DynDns, survey.WithValidator(survey.Required))
}

func configSelfSigned(config_obj *config_proto.Config) error {
	err := survey.Ask([]*survey.Question{
		{
			Name:     "Hostname",
			Prompt:   url_question,
			Validate: url_validator,
		},
		{
			Name:     "BindPort",
			Prompt:   port_question,
			Validate: port_validator,
		},
	}, config_obj.Frontend)

	if err != nil {
		return err
	}

	err = survey.Ask([]*survey.Question{
		{
			Name:     "BindPort",
			Validate: port_validator,
			Prompt:   gui_port_question,
		},
	}, config_obj.GUI)

	if err != nil {
		return err
	}

	config_obj.Client.UseSelfSignedSsl = true
	config_obj.Client.ServerUrls = append(
		config_obj.Client.ServerUrls,
		fmt.Sprintf("https://%s:%d/", config_obj.Frontend.Hostname,
			config_obj.Frontend.BindPort))

	config_obj.GUI.Authenticator = &config_proto.Authenticator{
		Type: "Basic"}

	return err
}

func configAutocert(config_obj *config_proto.Config) error {
	err := survey.Ask([]*survey.Question{{
		Name:     "Hostname",
		Validate: url_validator,
		Prompt:   url_question,
	},
	}, config_obj.Frontend)
	if err != nil {
		return err
	}

	// In autocert mode these are all fixed.
	config_obj.Frontend.BindPort = 443
	config_obj.Frontend.BindAddress = "0.0.0.0"

	// The gui is also served from port 443.
	config_obj.GUI.BindPort = 443
	config_obj.GUI.PublicUrl = fmt.Sprintf(
		"https://%s/", config_obj.Frontend.Hostname)

	config_obj.Client.ServerUrls = []string{
		fmt.Sprintf("https://%s/", config_obj.Frontend.Hostname)}

	config_obj.AutocertCertCache = config_obj.Datastore.Location

	return nil
}

func addUser(config_obj *config_proto.Config) error {
	for {
		username := ""
		err := survey.AskOne(user_name_question, &username, nil)
		if err != nil {
			fmt.Printf("%v", err)
			continue
		}

		if username == "" {
			return nil
		}

		user_record, err := users.NewUserRecord(username)
		if err != nil {
			fmt.Printf("%v", err)
			continue
		}

		auth_type := config_obj.GUI.Authenticator.Type

		if auth_type != "Basic" {
			fmt.Printf("Authentication will occur via %v - "+
				"therefore no password needs to be set.",
				auth_type)
		} else {
			password := ""
			err := survey.AskOne(password_question, &password,
				survey.WithValidator(survey.Required))
			if err != nil {
				fmt.Printf("%v", err)
				continue
			}

			users.SetPassword(user_record, password)
		}
		config_obj.GUI.InitialUsers = append(
			config_obj.GUI.InitialUsers,
			&config_proto.GUIUser{
				Name:         user_record.Name,
				PasswordHash: hex.EncodeToString(user_record.PasswordHash),
				PasswordSalt: hex.EncodeToString(user_record.PasswordSalt),
			})
	}
}
