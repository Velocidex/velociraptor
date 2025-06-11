package networking

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/third_party/cache"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	errRejectedThumbprint = errors.New("Server certificate had no known thumbprint")

	// To understand if caching client sessions will be useful we add
	// this count.
	metricClientSessionCacheMiss = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "tls_client_session_cache_miss",
			Help: "Count of how many client session cache misses we had",
		})
)

// hashCertificate takes a tls.Certificate and return the sha256
// fingerprint of said certificate. The return value is the hex
// representation of the byte sequence returned by the hashing
// function.
func hashCertificate(cert *x509.Certificate) string {
	h := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(h[:])
}

func normalizeThumbPrints(thumbprints []string) []string {
	thumbprintList := make([]string, 0, len(thumbprints))

	for _, thumbprint := range thumbprints {
		thumbprint = strings.ReplaceAll(thumbprint, ":", "") // ignore colons
		thumbprint = strings.ToLower(thumbprint)             // only use lowercase hash strings
		thumbprintList = append(thumbprintList, thumbprint)
	}
	return thumbprintList
}

// If we deployed Velociraptor using self signed certificates we want
// to be able to trust our own server. Our own server is signed by our
// own CA and also may have a different common name (not related to
// DNS). For example, in self signed mode, the server certificate is
// signed for "VelociraptorServer" but may be served over
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

	// Add a single root - the Velociraptor CA is the one we trust the most!
	if config_obj != nil {
		private_opts.Roots.AppendCertsFromPEM([]byte(config_obj.CaCertificate))
	}

	// this shouldn't be done for each connection attempt but currently
	// there does not seem to be a way to store the modified hash list
	thumbprintList := normalizeThumbPrints(config_obj.GetCrypto().GetCertificateThumbprints())
	verificationMode := strings.ToUpper(config_obj.GetCrypto().GetCertificateVerificationMode())

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

				switch verificationMode {
				// Strict enforcement - Only allow certificates
				// with this thumbprint exactly.
				case "THUMBPRINT_ONLY":
					if utils.InString(thumbprintList, hashCertificate(cert)) {
						return nil
					}
					return errRejectedThumbprint

					// Thumbprint enforcement is optional - if the
					// thumbprint matches we allow the connection in
					// any case.
				case "PKI_OR_THUMBPRINT":
					// Short circuit if the thumbprint matches
					// immediately
					if utils.InString(thumbprintList, hashCertificate(cert)) {
						return nil
					}
					// No thumbprint match here, verify as in PkiOnly
					fallthrough

				case "", "PKI":
					// If the server certificate is signed by the
					// Velociraptor CA (self signed mode) then we
					// allow it regardless of any other checks
					// (e.g. DNS check).

					// Velociraptor does not allow intermediates so
					// this should be sufficient to verify that the
					// Velociraptor CA signed it.
					_, err := server_cert.Verify(private_opts)
					if err == nil {
						// The Velociraptor CA signed it - we
						// disregard the DNS name and allow it anyway
						// - This allows us to connect to the
						// Velociraptor server by IP address.
						return nil
					}
				}

				// Continue to build an intermediate chain and proceed
				// to normal PKI verification.
			} else {
				public_opts.Intermediates.AddCert(cert)
			}
		}

		if server_cert == nil {
			return errors.New("Unknown server cert")
		}

		// Perform normal verification.
		_, err := server_cert.Verify(public_opts)
		if err != nil {
			return fmt.Errorf("While verifying %v: %w", server_cert.Subject.String(), err)
		}

		return err
	}
}

type ClientSessionCache struct {
	lru *cache.LRUCache
}

func (self *ClientSessionCache) Size() int {
	return 1
}

func (self *ClientSessionCache) Put(sessionKey string, cs *tls.ClientSessionState) {
	self.lru.Set(sessionKey, self)
}

func (self *ClientSessionCache) Get(sessionKey string) (*tls.ClientSessionState, bool) {
	_, ok := self.lru.Get(sessionKey)
	if !ok {
		metricClientSessionCacheMiss.Inc()
	}
	return nil, false
}

func GetTlsConfig(config_obj *config_proto.ClientConfig, extra_roots string) (*tls.Config, error) {
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

	if extra_roots != "" {
		if !CA_Pool.AppendCertsFromPEM([]byte(extra_roots)) {
			return nil, errors.New("Unable to parse root CA")
		}
	}

	result := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// This seems incompatible with multiple connections and
		// results in TLS errors. We need to consider if it is worth
		// it by using these metrics.
		ClientSessionCache: &ClientSessionCache{
			lru: cache.NewLRUCache(100),
		},
		RootCAs: CA_Pool,

		// Not actually skipping, we check the
		// cert in VerifyConnection
		InsecureSkipVerify: true,
		VerifyConnection:   customVerifyConnection(CA_Pool, config_obj),
	}

	// Automatically add any client certificates specified in the
	// config file. They will only be used when requested by the
	// server. NOTE: This could allow for fingerprinting the client by
	// MITM... This is necessary when connecting back to a mTLS
	// protected server to download tools.
	if config_obj.Crypto != nil &&
		config_obj.Crypto.ClientCertificate != "" &&
		config_obj.Crypto.ClientCertificatePrivateKey != "" {
		cert, err := tls.X509KeyPair(
			[]byte(config_obj.Crypto.ClientCertificate),
			[]byte(config_obj.Crypto.ClientCertificatePrivateKey))
		if err != nil {
			return nil, err
		}
		result.Certificates = []tls.Certificate{cert}
	}

	return result, nil
}

// GetSkipVerifyTlsConfig returns a config object where TLS verification is
// disabled if the client configuration allows it.
func GetSkipVerifyTlsConfig(config_obj *config_proto.ClientConfig) (*tls.Config, error) {
	c, err := GetTlsConfig(config_obj, "")
	if err != nil {
		return nil, err
	}

	if err = EnableSkipVerify(c, config_obj); err != nil {
		return nil, err
	}

	return c, nil
}

// If the TLS Verification policy allows it, enable SkipVerify to
// allow connections to invalid TLS servers.
func EnableSkipVerify(tlsConfig *tls.Config, config_obj *config_proto.ClientConfig) error {
	if tlsConfig == nil {
		return nil
	}

	if strings.ToUpper(config_obj.GetCrypto().GetCertificateVerificationMode()) == "THUMBPRINT_ONLY" {
		return errSkipVerifyDenied
	}

	tlsConfig.InsecureSkipVerify = true
	// Remove the custom verification - there will be no verification
	// because InsecureSkipVerify is true.
	tlsConfig.VerifyConnection = nil

	return nil
}
