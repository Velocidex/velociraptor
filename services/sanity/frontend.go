package sanity

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	basePathRegEx = regexp.MustCompile("^/[^/][a-zA-Z0-9_-]+[^/]$")
)

func (self *SanityChecks) CheckFrontendSettings(
	config_obj *config_proto.Config) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	if config_obj.Frontend != nil && config_obj.GUI != nil {
		// Check that certificates are valid.
		err := self.CheckCertificates(config_obj)
		if err != nil {
			return err
		}

		// Validate Allowed CIDRs
		for _, cidr := range config_obj.GUI.AllowedCidr {
			_, cidr_net, err := net.ParseCIDR(cidr)
			if err != nil {
				return fmt.Errorf("Invalid CIDR Range %v for GUI.allowed_cidr", cidr)
			}
			logger.Info("GUI Will only accept conections from <green>%v</>", cidr_net)
		}

		if config_obj.GUI.BasePath != "" {
			if !basePathRegEx.MatchString(config_obj.GUI.BasePath) {
				return fmt.Errorf("Invalid GUI.base_path. This must start with a / and end without a /. For example '/velociraptor' . Only a-z0-9 characters are allowed in the path name.")
			}

			if !strings.HasSuffix(config_obj.GUI.PublicUrl,
				config_obj.GUI.BasePath+"/app/index.html") {
				return fmt.Errorf("Invalid GUI.public_url. When setting a base_url the public_url must be adjusted accordingly. For example `https://www.example.com/velociraptor/app/index.html` for a base_path of `/velociraptor` .")
			}
		}

	}
	return nil
}
