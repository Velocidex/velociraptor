package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/config"
	//	utils_ "www.velocidex.com/golang/velociraptor/testing"
)

func parseRsaPrivateKeyFromPemStr(pem_str []byte) (*rsa.PrivateKey, error) {
	for {
		block, rest := pem.Decode(pem_str)
		if block == nil {
			return nil, errors.New("failed to parse PEM block containing the key")
		}

		if block.Type == "RSA PRIVATE KEY" {
			priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return nil, err
			}

			return priv, nil
		}
		pem_str = rest
	}
}

func parseX509CertFromPemStr(pem_str []byte) (*x509.Certificate, error) {
	for {
		block, rest := pem.Decode(pem_str)
		if block == nil {
			return nil, errors.New("Unable to parse PEM")
		}

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}

			return cert, nil
		}
		pem_str = rest
	}
}

func parseX509CSRFromPemStr(pem_str []byte) (*x509.CertificateRequest, error) {
	for {
		block, rest := pem.Decode(pem_str)
		if block == nil {
			return nil, errors.New("Unable to parse PEM")
		}

		if block.Type == "CERTIFICATE REQUEST" {
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			if err != nil {
				return nil, err
			}

			return csr, nil
		}
		pem_str = rest
	}
}

/* GRR Client IDs are derived from the public key of the client.

This makes it impossible to impersonate a client unless one has the
client's corresponding private key.

*/
func ClientIDFromPublicKey(public_key *rsa.PublicKey) string {
	raw_n := public_key.N.Bytes()
	result := make([]byte, 4+1+len(raw_n))
	binary.BigEndian.PutUint32(result[0:], uint32(len(raw_n)+1))
	copy(result[5:], raw_n)
	hashed := sha256.Sum256(result)
	dst := make([]byte, hex.EncodedLen(8))
	hex.Encode(dst, hashed[:8])
	return "C." + string(dst)
}

func GeneratePrivateKey() ([]byte, error) {
	reader := rand.Reader
	bitSize := 2048

	key, err := rsa.GenerateKey(reader, bitSize)
	if err != nil {
		return nil, err
	}
	pemdata := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		},
	)
	return pemdata, nil
}

// Verify the configuration, possibly updating default settings.
func VerifyConfig(config_obj *config.Config) error {
	if len(config_obj.Client_server_urls) == 0 {
		return errors.New("No server URLs configured!")
	}

	if config_obj.Client_private_key == nil {
		fmt.Println("Genering new private key....")
		pem, err := GeneratePrivateKey()
		if err != nil {
			return err
		}
		config_obj.Client_private_key = proto.String(string(pem))

		if config_obj.Config_writeback != nil {
			write_back_config := config.Config{}
			config.LoadConfig(*config_obj.Config_writeback, &write_back_config)
			write_back_config.Client_private_key = proto.String(string(pem))
			err = config.WriteConfigToFile(*config_obj.Config_writeback,
				&write_back_config)
			if err != nil {
				return err
			}
			fmt.Println("Wrote new config file ", *config_obj.Config_writeback)
		}
	}

	return nil
}
