package survey

import (
	"github.com/charmbracelet/huh"
)

func getSelfSignedConfig(config *ConfigSurvey) error {
	config.FrontendBindPort = "8000"
	config.GUIBindPort = "8889"

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Description("Configuring a Self Signed Server"),
		),
	).WithTheme(getTheme())

	return form.Run()

}
