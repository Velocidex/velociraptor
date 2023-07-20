package sanity

import (
	"errors"
	"fmt"
	"net"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

func (self *SanityChecks) CheckFrontendSettings(
	config_obj *config_proto.Config) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	if config_obj.Frontend != nil && config_obj.GUI != nil {
		// Validate Allowed CIDRs
		for _, cidr := range config_obj.GUI.AllowedCidr {
			_, cidr_net, err := net.ParseCIDR(cidr)
			if err != nil {
				return fmt.Errorf("Invalid CIDR Range %v for GUI.allowed_cidr", cidr)
			}
			logger.Info("GUI Will only accept conections from <green>%v</>", cidr_net)
		}

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
