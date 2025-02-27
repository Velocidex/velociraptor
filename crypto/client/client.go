package client

import (
	"context"
	"crypto/rsa"
	"crypto/x509"

	"github.com/Velocidex/ttlcache/v2"
	"github.com/go-errors/errors"
	"golang.org/x/time/rate"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ClientCryptoManager struct {
	CryptoManager
}

// Adds the server certificate to the crypto manager.
func (self *ClientCryptoManager) AddCertificate(
	config_obj *config_proto.Config,
	certificate_pem []byte) (string, error) {
	server_cert, err := crypto_utils.ParseX509CertFromPemStr(certificate_pem)
	if err != nil {
		return "", err
	}

	if server_cert.PublicKeyAlgorithm != x509.RSA {
		return "", errors.New("Not RSA algorithm")
	}

	// Verify that the certificate is signed by the CA.
	opts := x509.VerifyOptions{
		Roots:       self.caPool,
		CurrentTime: utils.GetTime().Now(),
	}

	_, err = server_cert.Verify(opts)
	if err != nil {
		return "", err
	}

	server_name := crypto_utils.GetSubjectName(server_cert)
	err = self.Resolver.SetPublicKey(
		config_obj, server_name, server_cert.PublicKey.(*rsa.PublicKey))
	if err != nil {
		return "", err
	}

	// Remove the cached key for this server. This is essential to
	// ensure servers can rotate their keys.
	self.cipher_lru.DeleteCipher(server_name)

	return server_name, nil
}

func NewClientCryptoManager(
	ctx context.Context,
	config_obj *config_proto.Config, client_private_key_pem []byte) (
	*ClientCryptoManager, error) {
	private_key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr(client_private_key_pem)
	if err != nil {
		return nil, err
	}

	logger := logging.GetLogger(config_obj, &logging.ClientComponent)
	client_id := crypto_utils.ClientIDFromPublicKey(&private_key.PublicKey)
	logger.Info("Starting Crypto for client %v", client_id)

	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))
	if !ok {
		return nil, errors.New("failed to parse CA certificate")
	}

	lru_size := int64(100)
	if config_obj.Frontend != nil &&
		config_obj.Frontend.Resources != nil {
		lru_size = config_obj.Frontend.Resources.ExpectedClients
	}

	limit_rate := int64(100)
	if config_obj.Frontend != nil &&
		config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.EnrollmentsPerSecond > 0 {
		limit_rate = config_obj.Frontend.Resources.EnrollmentsPerSecond
	}

	result := &ClientCryptoManager{CryptoManager{
		client_id:           client_id,
		private_key:         private_key,
		Resolver:            NewInMemoryPublicKeyResolver(),
		cipher_lru:          NewCipherLRU(lru_size),
		unauthenticated_lru: ttlcache.NewCache(),
		caPool:              roots,
		logger:              logger,
		limiter:             rate.NewLimiter(rate.Limit(limit_rate), 100),
	}}

	go func() {
		<-ctx.Done()
		result.unauthenticated_lru.Close()
	}()

	return result, nil
}
