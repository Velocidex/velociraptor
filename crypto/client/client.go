package client

import (
	"crypto/rsa"
	"crypto/x509"

	"github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/logging"
)

type ClientCryptoManager struct {
	CryptoManager
}

// Adds the server certificate to the crypto manager.
func (self *ClientCryptoManager) AddCertificate(certificate_pem []byte) (string, error) {
	server_cert, err := crypto_utils.ParseX509CertFromPemStr(certificate_pem)
	if err != nil {
		return "", err
	}

	if server_cert.PublicKeyAlgorithm != x509.RSA {
		return "", errors.New("Not RSA algorithm")
	}

	// Verify that the certificate is signed by the CA.
	opts := x509.VerifyOptions{
		Roots: self.caPool,
	}

	_, err = server_cert.Verify(opts)
	if err != nil {
		return "", err
	}

	server_name := crypto_utils.GetSubjectName(server_cert)
	err = self.Resolver.SetPublicKey(
		server_name, server_cert.PublicKey.(*rsa.PublicKey))
	if err != nil {
		return "", err
	}

	return server_name, nil
}

func NewClientCryptoManager(config_obj *config_proto.Config, client_private_key_pem []byte) (
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
	if config_obj.Frontend != nil {
		lru_size = config_obj.Frontend.Resources.ExpectedClients
	}

	return &ClientCryptoManager{CryptoManager{
		config:      config_obj,
		ClientId:    client_id,
		private_key: private_key,
		source:      client_id,
		Resolver:    NewInMemoryPublicKeyResolver(),
		cipher_lru:  NewCipherLRU(lru_size),
		caPool:      roots,
		logger:      logger,
	}}, nil
}
