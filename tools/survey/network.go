package survey

import (
	"errors"

	"github.com/charmbracelet/huh"
)

func getNetworkConfig(config *ConfigSurvey) error {

	items := []huh.Field{
		huh.NewNote().Description(
			"This section configures the server's network parameters."),
		huh.NewInput().
			Title("What is the public DNS name of the Master Frontend?").
			Placeholder("www.example.com").
			Description("Clients will connect to the Frontend using this public name.").
			Validate(func(in string) error {
				if url_validator.MatchString(in) {
					return nil
				}
				return errors.New("Invalid Hostname format (only allowed characters from this set [a-z0-9.A-Z-])")
			}).
			Value(&config.Hostname),

		huh.NewSelect[string]().
			Title("DNS Type").
			Description("In order for the server to be reachable from the internet, you must have DNS configured.").
			Options(
				huh.NewOption("None - Configure DNS manually", "none"),
				huh.NewOption("NOIP", "noip"),
				huh.NewOption("CloudFlare", "cloudflare"),
			).
			Value(&config.DynDNSType),

		huh.NewConfirm().
			Title("Would you like to try the new experimental websocket comms?").
			Description(`Websocket is a bidirectional low latency communication protocol supported by
most modern proxies and load balancers. This method is more efficient and
portable than plain HTTP. Be sure to test this in your environment.
`).
			Value(&config.UseWebsocket),
	}

	// If in self_signed mode we are allowed to configure the ports.
	if config.DeploymentType == "self_signed" {
		items = append(items, huh.NewInput().
			Title("Enter the frontend port to listen on.").
			Validate(validate_int("Frontend Port Number")).
			Value(&config.FrontendBindPort))

		items = append(items, huh.NewInput().
			Title("Enter the port for the GUI to listen on.").
			Validate(validate_int("GUI Port Number")).
			Value(&config.GUIBindPort))
	}

	form := huh.NewForm(huh.NewGroup(items...)).WithTheme(getTheme())
	err := form.Run()
	if err != nil {
		return err
	}

	questions := configureDynDNS(config)
	if questions == nil {
		return nil
	}

	form = huh.NewForm(huh.NewGroup(questions...)).WithTheme(getTheme())
	return form.Run()
}

func configureDynDNS(config *ConfigSurvey) []huh.Field {
	switch config.DynDNSType {
	case "none", "":
		return nil

	case "noip":
		return []huh.Field{
			huh.NewInput().
				Title("NoIP DynDNS Username").
				Validate(required("NoIP DynDNS Username")).
				Value(&config.DdnsUsername),
			huh.NewInput().
				Title("NoIP DynDNS Password").
				Validate(required("NoIP DynDNS Password")).
				Value(&config.DdnsPassword),
		}
	case "cloudflare":
		return []huh.Field{
			huh.NewInput().
				Title("Cloudflare Zone Name").
				Validate(required("Cloudflare Zone Name")).
				Value(&config.ZoneName),
			huh.NewInput().
				Title("Cloudflare API Token").
				Validate(required("Cloudflare API Token")).
				Value(&config.ApiToken),
		}
	}

	return nil
}
