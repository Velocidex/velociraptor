package crypto

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
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
