package survey

import (
	"path"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func getServerConfig(config *ConfigSurvey) error {
	theme := huh.ThemeBase16()
	theme.Focused.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF")).Bold(true)
	theme.Focused.Description = lipgloss.NewStyle().Foreground(lipgloss.Color("#DCDCDC"))
	theme.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	theme.Focused.FocusedButton = theme.Focused.FocusedButton.Foreground(lipgloss.Color("0")).Background(lipgloss.Color("2"))
	theme.Focused.BlurredButton = theme.Focused.BlurredButton.Foreground(lipgloss.Color("8"))
	theme.Blurred.BlurredButton = theme.Focused.BlurredButton.Foreground(lipgloss.Color("8"))
	theme.Blurred.FocusedButton = theme.Blurred.BlurredButton
	theme.Focused.SelectSelector = lipgloss.NewStyle().SetString(" ● ").Foreground(lipgloss.Color("2"))
	theme.Blurred.SelectSelector = lipgloss.NewStyle().SetString("   ")
	theme.Help.Ellipsis = theme.Help.Ellipsis.Foreground(lipgloss.Color("3"))
	theme.Help.ShortKey = theme.Help.ShortKey.Foreground(lipgloss.Color("3"))
	theme.Help.ShortDesc = theme.Help.ShortDesc.Foreground(lipgloss.Color("3"))
	theme.Help.ShortSeparator = theme.Help.ShortSeparator.Foreground(lipgloss.Color("3"))
	theme.Help.FullKey = theme.Help.FullKey.Foreground(lipgloss.Color("3"))
	theme.Help.FullDesc = theme.Help.FullDesc.Foreground(lipgloss.Color("3"))
	theme.Help.FullSeparator = theme.Help.FullSeparator.Foreground(lipgloss.Color("3"))
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
			huh.NewNote().
				Description(`The Datastore is where the server stores files.

It should be placed on a partitian large enough to contain all data you are likely to collect.`),
			huh.NewInput().
				Title("Path to the datastore directory.").
				Description("The datastore directory is where Velociraptor will store all files. Make sure there is sufficient disk space available!").
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

If you expect this deployment to exist part one year you might
consider extending the default validation.`).
				Options(
					huh.NewOption("1 Year", "1"),
					huh.NewOption("2 Years", "2"),
					huh.NewOption("10 Years", "10"),
				).
				Value(&config.CertExpiration),

			huh.NewConfirm().
				Title("Do you want to restrict VQL functionality on the server?").
				Description(`
This is useful for a shared server where users are not fully trusted.
It removes potentially dangerous plugins like execve(), filesystem access etc.

NOTE: This is an experimental feature only useful in limited situations. If you do not know you need it select N here!
`).
				Value(&config.ImplementAllowList),

			huh.NewConfirm().
				Title("Use registry for client writeback?").
				Description(`Traditionally Velociraptor uses files to store client state on all operating systems.

You can instead use the registry on Windows. NOTE: It is your responsibility to ensure the registry keys used are properly secured!

By default we use HKLM\SOFTWARE\Velocidex\Velociraptor
`).
				Value(&config.UseRegistryWriteback),
		),
	).WithTheme(theme)

	return form.Run()
}
