package frontend

import (
	"fmt"
	"net/url"
	"path"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

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

func (self *MasterFrontendManager) GetPublicUrl(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return GetPublicUrl(config_obj)
}

func (self MinionFrontendManager) GetPublicUrl(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return GetPublicUrl(config_obj)
}

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

func (self *MasterFrontendManager) GetBaseURL(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return GetBaseURL(config_obj)
}

func (self *MinionFrontendManager) GetBaseURL(
	config_obj *config_proto.Config) (res *url.URL, err error) {
	return GetBaseURL(config_obj)
}
