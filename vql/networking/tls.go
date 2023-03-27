package networking

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
)

// hashCertificate takes a tls.Certificate and return the sha256
// fingerprint of said certificate. The return value is the hex
// representation of the byte sequence returned by the hashing
// function.
func hashCertificate(cert *x509.Certificate) (string, error) {
	h := sha256.New()

	if _, err := h.Write(cert.Raw); err != nil {
		return "", err
	}

	checksum := h.Sum(nil)
	return hex.EncodeToString(checksum), nil
}

type VerificationMode int

const (
	UnknownMode VerificationMode = iota
	PkiOnly
	PkiOrThumbprint
	ThumbprintOnly
)

func convertVerificationMode(s string) VerificationMode {
	switch strings.ToUpper(s) {
	case "", "PKI":
		return PkiOnly

	case "PKI_OR_THUMBPRINT":
		return PkiOrThumbprint

	case "THUMBPRINT_ONLY":
		return ThumbprintOnly
	}

	return UnknownMode
}

// If we deployed Velociraptor using self signed certificates we want
// to be able to trust our own server. Our own server is signed by our
// own CA and also may have a different common name (not related to
// DNS). For example, in self signed mode, the server certificate is
// signed for VelociraptorServer but may be served over
// "localhost". Using the default TLS configuration this connection
// will be rejected.

// Therefore in the special case where the server cert is signed by
// our own CA, and the Subject name is the pinned server name
// (VelociraptorServer), we do not need to compare the server's common
// name with the url.

// This function is based on
// https://go.dev/src/crypto/tls/handshake_client.go::verifyServerCertificate
func customVerifyConnection(
	CA_Pool *x509.CertPool,
	config_obj *config_proto.ClientConfig) func(conn tls.ConnectionState) error {

	// Check if the cert was signed by the Velociraptor CA
	private_opts := x509.VerifyOptions{
		CurrentTime:   time.Now(),
		Intermediates: x509.NewCertPool(),
		Roots:         x509.NewCertPool(),
	}
	if config_obj != nil {
		private_opts.Roots.AppendCertsFromPEM([]byte(config_obj.CaCertificate))
	}

	// this shouldn't be done for each connection attempt but currently
	// there does not seem to be a way to store the modified hash list
	origThumbprints := config_obj.GetCrypto().GetCertificateThumbprints()
	thumbprintList := make([]string, 0, len(origThumbprints))

	for _, thumbprint := range origThumbprints {
		thumbprint = strings.ReplaceAll(thumbprint, ":", "") // ignore colons
		thumbprint = strings.ToLower(thumbprint)             // only use lowercase hash strings
		thumbprintList = append(thumbprintList, thumbprint)
	}

	verificationMode := convertVerificationMode(config_obj.GetCrypto().GetCertificateVerificationMode())

	return func(conn tls.ConnectionState) error {
		// Used to verify certs using public roots
		public_opts := x509.VerifyOptions{
			CurrentTime:   time.Now(),
			Intermediates: x509.NewCertPool(),
			DNSName:       conn.ServerName,
			Roots:         CA_Pool,
		}

		// First parse all the server certs so we can verify them. The
		// server presents its main cert first, then any following
		// intermediates.
		var server_cert *x509.Certificate

		for i, cert := range conn.PeerCertificates {
			// First cert is server cert.
			if i == 0 {
				server_cert = cert

				// If we're to verify thumbprints, we do that first so that
				// we can skip the rest of the certificate validation when
				// we found a known thumbprint
				if verificationMode > PkiOnly {
					certSha256, err := hashCertificate(cert)
					if err != nil {
						return err
					}

					for _, hash := range thumbprintList {
						if hash == certSha256 {
							// we found a matching thumbprint, connection can continue
							return nil
						}
					}

					// if certificate pinning is enforced, we need to abort the
					// connection when there was no match regardless of the fact
					// that a certificate may still be cryptographically valid
					if verificationMode >= ThumbprintOnly {
						return errors.New("no certificate in the chain had a known thumbprint")
					}
				}

				// Velociraptor does not allow intermediates so this
				// should be sufficient to verify that the
				// Velociraptor CA signed it.
				_, err := server_cert.Verify(private_opts)
				if err == nil {
					// The Velociraptor CA signed it - we disregard
					// the DNS name and allow it.
					return nil
				}

			} else {
				public_opts.Intermediates.AddCert(cert)
			}
		}

		if server_cert == nil {
			return errors.New("Unknown server cert")
		}

		// Perform normal verification.
		_, err := server_cert.Verify(public_opts)
		return err
	}
}

func GetTlsConfig(config_obj *config_proto.ClientConfig) (*tls.Config, error) {
	// Try to get the OS cert pool, failing that use a new one. We
	// already contain a list of valid root certs but using the OS
	// cert store allows us to support MITM proxies already on the
	// system. https://github.com/Velocidex/velociraptor/issues/2330
	CA_Pool, err := x509.SystemCertPool()
	if err != nil {
		CA_Pool = x509.NewCertPool()
	}
	err = crypto.AddDefaultCerts(config_obj, CA_Pool)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ClientSessionCache: tls.NewLRUClientSessionCache(100),
		RootCAs:            CA_Pool,

		// Not actually skipping, we check the
		// cert in VerifyPeerCertificate
		InsecureSkipVerify: true,
		NextProtos:         []string{"http/1.1"},
		VerifyConnection:   customVerifyConnection(CA_Pool, config_obj),
	}, nil
}
