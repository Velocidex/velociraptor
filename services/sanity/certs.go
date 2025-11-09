package sanity

import (
	"errors"
	"fmt"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Check if the frontend certificates have expired. In the past this
// was a hard error but many users encountered it without warning and
// were confused about how to deal with it. Now we try to recover by
// automatically rekeying the certs on start up.

// In more secure deployments we recommend removing the CA private key
// from the server config, which prevents us from automatically
// rekeying the certificates. See
// https://docs.velociraptor.app/docs/deployment/security/ for
// relevant discussion.
func (self *SanityChecks) CheckCertificates(
	config_obj *config_proto.Config) error {

	cert, err := crypto_utils.ParseX509CertFromPemStr(
		[]byte(config_obj.Frontend.Certificate))
	if err != nil {
		return err
	}

	now := utils.GetTime().Now()

	if cert.NotBefore.After(now) || cert.NotAfter.Before(now) {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		msg1 := fmt.Sprintf("<red>Frontend Certificate is not valid</>: Certificate Valid NotBefore %v and Not After %v but Now is %v. See https://docs.velociraptor.app/knowledge_base/tips/rolling_certificates/",
			cert.NotBefore.Format(time.RFC3339),
			cert.NotAfter.Format(time.RFC3339),
			now.Format(time.RFC3339),
		)
		logger.Error("%v", msg1)

		if config_obj.CA != nil && config_obj.CA.PrivateKey != "" {
			msg2 := fmt.Sprintf("<green>Found CA private key in config</>, I will automatically rotate keys, but you should consider updating the config file using `velociraptor config rotate_keys`")
			logger.Info("%s", msg2)

			frontend_cert, err := crypto.GenerateServerCert(
				config_obj, utils.GetSuperuserName(config_obj))
			if err != nil {
				return fmt.Errorf("Unable to create Frontend cert: %w", err)
			}

			config_obj.Frontend.Certificate = frontend_cert.Cert
			config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

			if config_obj.GUI != nil {
				// Generate gRPC gateway certificate.
				gw_certificate, err := crypto.GenerateServerCert(
					config_obj, utils.GetGatewayName(config_obj))
				if err != nil {
					return err
				}

				config_obj.GUI.GwCertificate = gw_certificate.Cert
				config_obj.GUI.GwPrivateKey = gw_certificate.PrivateKey
			}

			// Make sure the user knows about the issue!
			frontend_service, err := services.GetFrontendManager(config_obj)
			if err == nil {
				frontend_service.SetGlobalMessage(
					&api_proto.GlobalUserMessage{
						Key:     "ExpiredCert",
						Level:   "ERROR",
						Message: msg1,
					})
				frontend_service.SetGlobalMessage(
					&api_proto.GlobalUserMessage{
						Key:     "ExpiredCert2",
						Level:   "INFO",
						Message: msg2,
					})

			}
			return nil
		}

		return errors.New("Certificate not valid")
	}

	return nil
}
