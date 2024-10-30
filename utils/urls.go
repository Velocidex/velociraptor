package utils

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// Work around issues with https://github.com/golang/go/issues/4013
// and space encoding. This QueryEscape has to be the exact mirror of
// Javascript's decodeURIComponent
func QueryEscape(in string) string {
	res := url.QueryEscape(in)
	return strings.Replace(res, "+", "%20", -1)
}

// The URL to the App.html itself
func GetPublicUrl(config_obj *config_proto.Config) (res *url.URL, err error) {
	res = &url.URL{Path: "/"}

	if config_obj.GUI != nil && config_obj.GUI.PublicUrl != "" {
		res, err = url.Parse(config_obj.GUI.PublicUrl)
		if err != nil {
			return nil, fmt.Errorf(
				"Invalid configuration! GUI.public_url must be the public URL over which the GUI is served!: %w", err)
		}
	}
	res.RawQuery = ""
	res.Fragment = ""
	res.RawFragment = ""

	return res, nil
}

// Calculates the Base URL to the top of the app
func GetBaseURL(config_obj *config_proto.Config) (res *url.URL, err error) {
	res, err = GetPublicUrl(config_obj)
	if err != nil {
		return nil, err
	}
	res.Path = "/"
	if config_obj.GUI != nil && config_obj.GUI.BasePath != "" {
		res.Path = path.Join(res.Path, config_obj.GUI.BasePath)
	}

	return res, nil
}
