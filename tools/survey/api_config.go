package survey

import "github.com/charmbracelet/huh"

func GetAPIClientPassword() (string, error) {
	passwd := ""
	items := []huh.Field{
		huh.NewInput().
			Title("Enter password to encrypt the API key").
			Description(`
The API key is written in PEM format in the api configuration file. You can add a password to require it to be unlocked before use. This is not suitable for automated access to the API but is an additional security measure for interactive access.`).
			Placeholder("Enter a password").
			EchoMode(huh.EchoModePassword).
			Value(&passwd),
	}

	form := huh.NewForm(huh.NewGroup(items...)).WithTheme(getTheme())
	err := form.Run()
	return passwd, err
}

func GetAPIClientDecryptPassword() (string, error) {
	passwd := ""
	items := []huh.Field{
		huh.NewInput().
			Title("Enter password to unlock the API key").
			Placeholder("Enter a password").
			EchoMode(huh.EchoModePassword).
			Value(&passwd),
	}

	form := huh.NewForm(huh.NewGroup(items...)).WithTheme(getTheme())
	err := form.Run()
	return passwd, err
}
