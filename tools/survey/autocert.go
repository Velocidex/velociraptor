package survey

import (
	"github.com/charmbracelet/huh"
)

func getAutoCertConfig(config *ConfigSurvey) error {
	config.FrontendBindPort = "443"
	config.GUIBindPort = "443"

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Description("Configuring Server certificates using Let's Encrypt."),
		),
	)

	return form.Run()
}
