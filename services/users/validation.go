package users

import (
	"errors"
	"fmt"
	"strings"

	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

type Validator struct{}

func (self Validator) validateLang(lang string) (string, error) {
	lang = strings.ToLower(lang)
	switch lang {
	case "english":
		return "en", nil

	case "german":
		return "de", nil

	case "spanish":
		return "es", nil

	case "portugese":
		return "por", nil

	case "french":
		return "fr", nil

	case "japanese":
		return "jp", nil

	case "vietnamese":
		return "vi", nil

	case "en", "de", "es", "por", "fr", "jp", "vi":
		return lang, nil
	default:
		return "", errors.New(
			"Invalid language set. Can only be en, de, es, por, fr, jp, vi")
	}
}

func (self Validator) validateTheme(theme string) (string, error) {
	theme = strings.ToLower(theme)
	switch theme {
	case "veloci-light", "veloci-dark", "veloci-docs",
		"no-theme", "pink-light",
		"ncurses-light", "ncurses-dark",
		"github-dimmed-dark",
		"coolgray-dark", "midnight",
		"vscode-dark":
		return theme, nil

	default:
		return "", fmt.Errorf("Invalid theme %v. Can only be veloci-light, veloci-dark, veloci-docs, no-theme, pink-light, ncurses-light, ncurses-dark, github-dimmed-dark, coolgray-dark, midnight, vscode-dark", theme)
	}
}

// For now we dont validate this
func (self Validator) validateTimezone(tz string) (string, error) {
	return tz, nil
}

func (self Validator) validateOrg(org string) (string, error) {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return "", err
	}

	_, err = org_manager.GetOrg(org)
	return org, err
}

func (self Validator) validateLinks(
	config_obj *config_proto.Config, links []*config_proto.GUILink) (
	[]*config_proto.GUILink, error) {
	for idx, l := range links {
		if l.Text == "" {
			l.Text = fmt.Sprintf("Link %v", idx)
		}
		if l.Url == "" {
			return nil, fmt.Errorf("Link %v has no URL!", l.Text)
		}

		if l.Type == "" {
			l.Type = "sidebar"
		}

		// If no icon is specified we just use the default Velo icon
		// because why not?
		if l.IconUrl == "" {
			l.IconUrl = config.VeloIconDataURL
		}

		if l.Method == "" {
			l.Method = "GET"
		}

		if l.Method != "GET" && l.Method != "POST" {
			return nil, fmt.Errorf("Link %v method must be GET or POST!", l.Text)
		}
	}

	// Merge with the default links. If you want to hide the default
	// links, just add a link with the same Text field and set it to
	// Disabled.
	if config_obj.GUI != nil {
		links = MergeGUILinks(links, config_obj.GUI.Links)
	}

	links = MergeGUILinks(links, DefaultLinks)

	return links, nil
}
