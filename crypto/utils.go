/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"

	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
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
				return nil, errors.WithStack(err)
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
				return nil, errors.WithStack(err)
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
				return nil, errors.WithStack(err)
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
		return nil, errors.WithStack(err)
	}
	pemdata := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		},
	)
	return pemdata, nil
}

func PublicKeyToPem(key *rsa.PublicKey) []byte {
	return pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: x509.MarshalPKCS1PublicKey(key),
		},
	)
}

func PemToPublicKey(pem_str []byte) (*rsa.PublicKey, error) {
	for {
		block, rest := pem.Decode(pem_str)
		if block == nil {
			return nil, errors.New("failed to parse PEM block containing the key")
		}

		if block.Type == "RSA PUBLIC KEY" {
			pub, err := x509.ParsePKCS1PublicKey(block.Bytes)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			return pub, nil
		}
		pem_str = rest
	}
}

// Verify the configuration, possibly updating default settings.
func VerifyConfig(config_obj *api_proto.Config) error {
	if len(config_obj.Client.ServerUrls) == 0 {
		return errors.New("No server URLs configured!")
	}

	if config_obj.Writeback.PrivateKey == "" {
		fmt.Println("Genering new private key....")
		pem, err := GeneratePrivateKey()
		if err != nil {
			return errors.WithStack(err)
		}

		config_obj.Writeback.PrivateKey = string(pem)
		err = config.UpdateWriteback(config_obj)
		if err != nil {
			return err
		}
	}

	return nil
}
