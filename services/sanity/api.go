package sanity

import (
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
)

func (self *SanityChecks) CheckAPISettings(
	config_obj *config_proto.Config) error {

	// Make sure to fill in the pinned gateway name from the gateway's
	// certificate.
	if config_obj.GUI != nil &&
		config_obj.GUI.GwCertificate != "" {
		cert, err := crypto_utils.ParseX509CertFromPemStr([]byte(config_obj.GUI.GwCertificate))
		if err != nil {
			return fmt.Errorf("CheckAPISettings: While parsing GUI.gw_certificate: %w", err)
		}

		if config_obj.API == nil {
			config_obj.API = &config_proto.APIConfig{}
		}

		config_obj.API.PinnedGwName = crypto_utils.GetSubjectName(cert)
	}
	return nil
}
