package crypto

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
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
