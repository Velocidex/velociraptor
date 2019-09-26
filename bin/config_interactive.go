package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Velocidex/survey"
	"github.com/Velocidex/yaml"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/users"
)

const (
	self_signed = "Self Signed SSL"
	autocert    = "Automatically provision certificates with Lets Encrypt"
	oauth_sso   = "Authenticate users with Google OAuth SSO"
)

var (
	deployment_type = &survey.Select{
		Message: `
Welcome to the Velociraptor configuration generator
---------------------------------------------------

I will be creating a new deployment configuration for you. I will
begin by identifying what type of deployment you need.

`,
		Options: []string{self_signed, autocert, oauth_sso},
	}

	url_question = &survey.Input{
		Message: "What is the public DNS name of the Frontend " +
			"(e.g. www.example.com):",
		Help: "Clients will connect to the Frontend using this " +
			"public name (e.g. https://www.example.com:8000/ ).",
		Default: "localhost",
	}

	port_question = &survey.Input{
		Message: "Enter the frontend port to listen on.",
		Default: "8000",
	}

	data_store_question = &survey.Input{
		Message: "Path to the datastore directory.",
		Default: os.TempDir(),
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

	google_oauth_client_id_question = &survey.Input{
		Message: "Enter the Google OAuth Client ID?",
	}

	google_oauth_client_secret_question = &survey.Input{
		Message: "Enter the Google OAuth Client Secret?",
	}

	google_domains_username = &survey.Input{
		Message: "Google Domains DynDNS Username",
	}

	google_domains_password = &survey.Input{
		Message: "Google Domains DynDNS Password",
	}
)

func doGenerateConfigInteractive() {
	install_type := ""
	err := survey.AskOne(deployment_type, &install_type, nil)
	kingpin.FatalIfError(err, "Question")

	fmt.Println("Generating keys please wait....")
	config_obj, err := generateNewKeys()
	kingpin.FatalIfError(err, "Generating Keys")

	switch install_type {
	case self_signed:
		err = survey.AskOne(port_question, &config_obj.Frontend.BindPort, nil)
		kingpin.FatalIfError(err, "Question")

		hostname := ""
		err = survey.AskOne(url_question, &hostname, survey.WithValidator(survey.Required))
		kingpin.FatalIfError(err, "Question")

		config_obj.Client.ServerUrls = append(
			config_obj.Client.ServerUrls,
			fmt.Sprintf("https://%s:%d/", hostname,
				config_obj.Frontend.BindPort))

	case autocert:
		// In autocert mode these are all fixed.
		config_obj.Frontend.BindPort = 443
		config_obj.GUI.BindPort = 443
		config_obj.Frontend.BindAddress = "0.0.0.0"

		hostname := ""
		err = survey.AskOne(url_question, &hostname, survey.WithValidator(survey.Required))
		kingpin.FatalIfError(err, "Question")

		config_obj.Client.ServerUrls = []string{
			fmt.Sprintf("https://%s/", hostname)}

		config_obj.AutocertDomain = hostname
		config_obj.AutocertCertCache = config_obj.Datastore.Location

		err = dynDNSConfig(config_obj, hostname)
		kingpin.FatalIfError(err, "dynDNSConfig")

	case oauth_sso:
		// In autocert mode these are all fixed.
		config_obj.Frontend.BindPort = 443
		config_obj.GUI.BindPort = 443
		config_obj.Frontend.BindAddress = "0.0.0.0"

		hostname := ""
		err = survey.AskOne(url_question, &hostname, survey.WithValidator(survey.Required))
		kingpin.FatalIfError(err, "Question")

		config_obj.GUI.PublicUrl = fmt.Sprintf("https://%s/", hostname)
		config_obj.Client.ServerUrls = []string{config_obj.GUI.PublicUrl}

		config_obj.AutocertDomain = hostname
		config_obj.AutocertCertCache = config_obj.Datastore.Location

		err = survey.AskOne(google_oauth_client_id_question,
			&config_obj.GUI.GoogleOauthClientId,
			survey.WithValidator(survey.Required))
		kingpin.FatalIfError(err, "Question")

		err = survey.AskOne(google_oauth_client_secret_question,
			&config_obj.GUI.GoogleOauthClientSecret,
			survey.WithValidator(survey.Required))
		kingpin.FatalIfError(err, "Question")

		err = dynDNSConfig(config_obj, hostname)
		kingpin.FatalIfError(err, "dynDNSConfig")
	}

	err = getFileStoreLocation(config_obj)
	kingpin.FatalIfError(err, "getFileStoreLocation")

	err = getLogLocation(config_obj)
	kingpin.FatalIfError(err, "getLogLocation")

	path := ""
	err = survey.AskOne(output_question, &path, survey.WithValidator(survey.Required))
	kingpin.FatalIfError(err, "Question")

	res, err := yaml.Marshal(config_obj)
	kingpin.FatalIfError(err, "Yaml Marshal")

	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	kingpin.FatalIfError(err, "Open file %s", path)
	defer fd.Close()

	fd.Write(res)

	err = survey.AskOne(client_output_question, &path,
		survey.WithValidator(survey.Required))
	kingpin.FatalIfError(err, "Question")

	client_config := getClientConfig(config_obj)
	res, err = yaml.Marshal(client_config)
	kingpin.FatalIfError(err, "Yaml Marshal")

	fd, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	kingpin.FatalIfError(err, "Open file %s", path)
	defer fd.Close()

	fd.Write(res)

	kingpin.FatalIfError(addUser(config_obj), "Add users")
}

func dynDNSConfig(config_obj *config_proto.Config, hostname string) error {
	dyndns := false
	err := survey.AskOne(&survey.Confirm{
		Message: "Are you using Google Domains DynDNS?"},
		&dyndns, survey.WithValidator(survey.Required))
	if err != nil {
		return err
	}

	if dyndns {
		username := ""
		err = survey.AskOne(
			google_domains_username, &username,
			survey.WithValidator(survey.Required))
		if err != nil {
			return err
		}

		password := ""
		err = survey.AskOne(google_domains_password, &password,
			survey.WithValidator(survey.Required))
		if err != nil {
			return err
		}

		config_obj.Frontend.DynDns = &config_proto.DynDNSConfig{
			Hostname:     hostname,
			DdnsUsername: username,
			DdnsPassword: password,
		}
	}

	return nil
}

func getFileStoreLocation(config_obj *config_proto.Config) error {
	err := survey.AskOne(data_store_question,
		&config_obj.Datastore.Location,
		survey.WithValidator(func(val interface{}) error {
			// Check that the directory exists.
			stat, err := os.Stat(val.(string))
			if err == nil && stat.IsDir() {
				return nil
			}
			return err
		}))
	if err != nil {
		return err
	}

	config_obj.Datastore.FilestoreDirectory = config_obj.Datastore.Location

	// Put the public directory inside the file store.
	config_obj.Frontend.PublicPath = filepath.Join(config_obj.Datastore.Location,
		"public")

	return nil
}

func getLogLocation(config_obj *config_proto.Config) error {
	err := survey.AskOne(log_question,
		&config_obj.Logging.OutputDirectory,
		survey.WithValidator(func(val interface{}) error {
			// Check that the directory exists.
			stat, err := os.Stat(val.(string))
			if err == nil && stat.IsDir() {
				return nil
			}
			return err
		}))
	if err != nil {
		return err
	}

	config_obj.Logging.SeparateLogsPerComponent = true
	return nil
}

func addUser(config_obj *config_proto.Config) error {
	for {
		username := ""
		err := survey.AskOne(user_name_question, &username, nil)
		if err != nil {
			return err
		}

		if username == "" {
			return nil
		}

		user_record, err := users.NewUserRecord(username)
		if err != nil {
			return err
		}

		if config_obj.GUI.GoogleOauthClientId != "" {
			fmt.Printf("Authentication will occur via Google - " +
				"therefore no password needs to be set.")
		} else {
			password := ""
			err := survey.AskOne(password_question, &password,
				survey.WithValidator(survey.Required))
			if err != nil {
				return err
			}

			user_record.SetPassword(password)
		}

		err = users.SetUser(config_obj, user_record)
		if err != nil {
			return err
		}
	}
}
