package survey

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
)

func getSSORedirect(config *ConfigSurvey) string {
	res := fmt.Sprintf("https://%v/", config.Hostname)
	switch config.SSOType {
	case "google":
		return res + "auth/google/callback"
	case "github":
		return res + "auth/github/callback"
	case "azure":
		return res + "auth/azure/callback"
	case "oidc":
		return res + "auth/oidc/callback"
	}
	return ""
}

func configSSO(config *ConfigSurvey) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Description("Configuring the SSO Providers"),
			huh.NewSelect[string]().
				Title("Select the SSO Authentication Provider").
				Options(
					huh.NewOption("Google", "google"),
					huh.NewOption("GitHub", "github"),
					huh.NewOption("Azure", "azure"),
					huh.NewOption("OIDC", "oidc"),
				).
				Value(&config.SSOType),

			huh.NewNote().
				DescriptionFunc(func() string {
					return fmt.Sprintf(
						"Setting %v configuration will use redirect URL %v\n",
						config.SSOType, getSSORedirect(config))
				}, &config.SSOType),
		),
	).WithTheme(getTheme())

	err := form.Run()
	if err != nil {
		return err
	}

	fields := baseSSOGroup(config)

	switch config.SSOType {
	case "google", "github":
	case "azure":
		fields = append(fields, huh.NewInput().
			Title("Enter the Tenant Domain name or ID").
			Value(&config.AzureTenantID))

	case "oidc":
		fields = append(fields, huh.NewInput().
			Title("Enter valid OIDC Issuer URL").
			Description("e.g. https://accounts.google.com or https://your-org-name.okta.com are valid Issuer URLs, check that URL has /.well-known/openid-configuration endpoint").
			Validate(func(in string) error {
				// A check to avoid double slashes
				if len(in) == 0 {
					return errors.New("Must set value")
				}
				if in[len(in)-1:] == "/" {
					return fmt.Errorf("Issuer URL should not have / (slash) sign as the last symbol")
				}
				return nil
			}).
			Value(&config.OIDCIssuer))
	}

	form = huh.NewForm(huh.NewGroup(fields...)).WithTheme(getTheme())
	return form.Run()
}

func configAuth(config *ConfigSurvey) error {
	if config.DeploymentType == "oauth_sso" {
		err := configSSO(config)
		if err != nil {
			return err
		}
	}

	// Add users to the config
	for i := 0; i < 10; i++ {
		user_record := UserRecord{}

		items := []huh.Field{
			huh.NewNote().
				Title(fmt.Sprintf("Adding Admin User Number %v", i)).
				Description("Enter an empty username or Ctrl-C to stop"),
			huh.NewInput().
				Title("Username").
				Value(&user_record.Name),
		}

		// For regular deployments we add the password
		if config.DeploymentType != "oauth_sso" {
			items = append(items, huh.NewInput().
				Title("Password").
				Value(&user_record.Password))
		}

		form := huh.NewForm(huh.NewGroup(items...)).WithTheme(getTheme())
		err := form.Run()
		if err != nil || user_record.Name == "" {
			break
		}

		config.DefaultUsers = append(config.DefaultUsers, user_record)
	}

	return nil
}

func baseSSOGroup(config *ConfigSurvey) []huh.Field {
	return []huh.Field{
		huh.NewNote().
			DescriptionFunc(func() string {
				return fmt.Sprintf("Configure %v SSO Provider", config.SSOType)
			}, &config.SSOType),
		huh.NewInput().
			Title("Enter the OAuth Client ID").
			Value(&config.OauthClientId),
		huh.NewInput().
			Title("Enter the OAuth Client Secret").
			Value(&config.OauthClientSecret),
	}
}
