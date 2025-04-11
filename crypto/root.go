// Include the root CAs in the binary itself. This helps with older
// systems that may not have the latest CA information (e.g. the Let's
// Encrypt Root expired in Sep 2021).

// By default Golang will accept root certs from the SSL_CERT_FILE and
// SSL_CERT_DIR env variables. We do not allow that, requiring instead
// that root CAs be included in the config file only.

package crypto

import (
	"crypto/x509"

	errors "github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

// We support two distinct modes:

// 1. Self signed mode - means that Velociraptor will only trust it's
//    own CA to sign certs.
// 2. Public PKI mode - Velociraptor will trust only known root CAs to
//    sign. Root CA store is built into the binary in addition to the
//    system store.

// The mode is specified by the Client.use_self_signed_ssl flag in the
// configuration file.

// In either mode, Certs will be added from the configuration file's
// Client.Crypto.root_certs setting.

// Add Default roots: our own CA is a root because we always trust
// it. Also add any additional roots specified in the config file.
func AddDefaultCerts(
	config_obj *config_proto.ClientConfig, CA_Pool *x509.CertPool) error {

	if config_obj != nil {
		// Always trust ourselves anyway.
		CA_Pool.AppendCertsFromPEM([]byte(config_obj.CaCertificate))
	}

	// Now add any additional certs from the config file.
	if config_obj != nil &&
		config_obj.Crypto != nil &&
		config_obj.Crypto.RootCerts != "" {
		if !CA_Pool.AppendCertsFromPEM([]byte(config_obj.Crypto.RootCerts)) {
			return errors.New(
				"Unable to parse Crypto.root_certs in the config file.")
		}
	}
	return nil
}

func AddPublicRoots(CA_Pool *x509.CertPool) {
	data, err := utils.GzipUncompress(FileCryptoCaCertificatesCrt)
	if err != nil {
		// Cant really happen normally!
		panic(err)
	}
	CA_Pool.AppendCertsFromPEM(data)
}
