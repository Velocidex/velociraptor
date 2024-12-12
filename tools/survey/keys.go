package survey

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func GenerateNewKeys(config_obj *config_proto.Config) error {
	ca_bundle, err := crypto.GenerateCACert(2048)
	if err != nil {
		return fmt.Errorf("Unable to create CA cert: %w", err)
	}

	config_obj.Client.CaCertificate = ca_bundle.Cert
	config_obj.CA.PrivateKey = ca_bundle.PrivateKey

	nonce := make([]byte, 8)
	_, err = rand.Read(nonce)
	if err != nil {
		return fmt.Errorf("Unable to create nonce: %w", err)
	}
	config_obj.Client.Nonce = base64.StdEncoding.EncodeToString(nonce)

	// Make another nonce for VQL obfuscation.
	_, err = rand.Read(nonce)
	if err != nil {
		return fmt.Errorf("Unable to create nonce: %w", err)
	}
	config_obj.ObfuscationNonce = base64.StdEncoding.EncodeToString(nonce)

	// Generate frontend certificate. Frontend certificates must
	// have a constant common name - clients will refuse to talk
	// with another common name.
	frontend_cert, err := crypto.GenerateServerCert(
		config_obj, utils.GetSuperuserName(config_obj))
	if err != nil {
		return fmt.Errorf("Unable to create Frontend cert: %w", err)
	}

	config_obj.Frontend.Certificate = frontend_cert.Cert
	config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Generate gRPC gateway certificate.
	gw_certificate, err := crypto.GenerateServerCert(
		config_obj, utils.GetGatewayName(config_obj))
	if err != nil {
		return fmt.Errorf("Unable to create Frontend cert: %w", err)
	}

	config_obj.GUI.GwCertificate = gw_certificate.Cert
	config_obj.GUI.GwPrivateKey = gw_certificate.PrivateKey

	return nil
}
