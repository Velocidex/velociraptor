package survey

import (
	"path"

	"github.com/charmbracelet/huh"
)

func getServerConfig(config *ConfigSurvey) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Welcome to the Velociraptor configuration generator").
				Description(`
This wizard creates a configuration file for a new deployment.

Let's begin by configuring the server itself.
`),
			huh.NewSelect[string]().
				Title("Deployment Type").
				Description("This wizard can create the following distinct deployment types.").
				Options(
					huh.NewOption("Self Signed SSL", "self_signed"),
					huh.NewOption("Automatically provision certificates with Lets Encrypt", "autocert"),
					huh.NewOption("Authenticate users with SSO", "oauth_sso"),
				).
				Value(&config.DeploymentType),

			huh.NewSelect[string]().
				Title(`What OS will the server be deployed on?`).
				Description("For production use we recommend to deploy on Ubuntu Linux, but you can deploy on other platforms for testing.").
				Options(
					huh.NewOption("Linux", "linux"),
					huh.NewOption("Windows", "windows"),
					huh.NewOption("MacOS", "darwin"),
				).
				Value(&config.ServerType),
		),
		huh.NewGroup(
			// for some reason this note isn't being displayed
			huh.NewNote().
				Description("The datastore directory is where Velociraptor will store all files."),
			huh.NewInput().
				Title("Path to the datastore directory.").
				Description(`The datastore directory is where Velociraptor will store all files.
This should be located on a partitian large enough to contain all data you are likely to collect.
Make sure there is sufficient disk space available!`).
				PlaceholderFunc(func() string {
					return config.DefaultDatastoreLocation()
				}, &config.ServerType).
				Value(&config.DatastoreLocation),
			huh.NewInput().
				Title("Path to the logs directory.").
				Description("Velociraptor will write logs to this directory. By default it resides within the datastore directory but you can place it anywhere.").
				PlaceholderFunc(func() string {
					return path.Join(config.DatastoreLocation, "logs")
				}, &config.DatastoreLocation).
				Value(&config.LoggingPath),

			huh.NewSelect[string]().
				Title("Internal PKI Certificate Expiration").
				Description(`By default internal certificates are issued for 1 year.

If you expect this deployment to exist past one year you might
consider extending the default validation.`).
				Options(
					huh.NewOption("1 Year", "1"),
					huh.NewOption("2 Years", "2"),
					huh.NewOption("10 Years", "10"),
				).
				Value(&config.CertExpiration),

			// This causes too many problems with users who dont know
			// what this does and enable it. Taking it out of the
			// wizard should prevent these issues. Users can add this
			// later.

			/*
			   			huh.NewConfirm().
			   				Title("Do you want to restrict VQL functionality on the server?").
			   				Description(`
			   This is useful for a shared server where users are not fully trusted.
			   It removes potentially dangerous plugins like execve(), filesystem access etc.

			   NOTE: This is an experimental feature only useful in limited situations. If you do not know you need it select N here!
			   `).
			   				Value(&config.ImplementAllowList),
			*/

			huh.NewConfirm().
				Title("Use registry for client writeback?").
				Description(`Traditionally Velociraptor uses files to store client state on all operating systems.

You can instead use the registry on Windows. NOTE: It is your responsibility to ensure the registry keys used are properly secured!

By default we use HKLM\SOFTWARE\Velocidex\Velociraptor
`).
				Value(&config.UseRegistryWriteback),
		),
	).WithTheme(getTheme())

	return form.Run()
}
