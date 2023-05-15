package sanity

import (
	"errors"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func (self *SanityChecks) CheckFrontendSettings(
	config_obj *config_proto.Config) error {

	if config_obj.Frontend != nil && config_obj.GUI != nil {
		auther := config_obj.GUI.Authenticator
		if auther != nil && strings.ToLower(auther.Type) == "certs" {
			if config_obj.Frontend.BindPort == config_obj.GUI.BindPort {
				return errors.New(
					"'Certs' authenticator can not be used when frontend and GUI share the same port")
			}
			if config_obj.AutocertCertCache != "" {
				return errors.New(
					"'Certs' authenticator can not be used with autocert enabled")
			}
		}
	}
	return nil
}
