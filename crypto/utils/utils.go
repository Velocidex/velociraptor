/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"

	"github.com/go-errors/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/writeback"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Retrieve frontend private key frome scope
// Must be running on server
func GetPrivateKeyFromScope(scope vfilter.Scope) (*rsa.PrivateKey, error) {

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil, errors.New("Must be running on server!")
	}

	if config_obj.Frontend == nil {
		return nil, errors.New("No frontend configuration given")
	}
	private_key := config_obj.Frontend.PrivateKey

	key, err := ParseRsaPrivateKeyFromPemStr([]byte(private_key))
	if err != nil {
		return nil, err
	}
	return key, nil
}

// Decode base64 encoded data and decrypt RSA-OAEP
func Base64DecryptRSAOAEP(pk *rsa.PrivateKey, data string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	return DecryptRSAOAEP(pk, decoded)
}

// Decrypt RSA-OAEP
func DecryptRSAOAEP(pk *rsa.PrivateKey, data []byte) ([]byte, error) {
	hash := sha512.New()
	return rsa.DecryptOAEP(hash, rand.Reader, pk, data, nil)
}

// Encrypt data using public key from X509 certificate
func EncryptWithX509PubKey(msg []byte, cert *x509.Certificate) ([]byte, error) {
	pub := cert.PublicKey
	switch pub := pub.(type) {
	case *rsa.PublicKey:
		return EncryptRSAOAEP(msg, pub)
	default:
		return nil, errors.New("Unsupported Type of Public Key")
	}
}

// Encrypt data using RSA-OAEP
func EncryptRSAOAEP(msg []byte, pub *rsa.PublicKey) ([]byte, error) {
	hash := sha512.New()
	return rsa.EncryptOAEP(hash, rand.Reader, pub, msg, nil)
}

func ParseRsaPrivateKeyFromPemStr(pem_str []byte) (*rsa.PrivateKey, error) {
	for {
		block, rest := pem.Decode(pem_str)
		if block == nil {
			return nil, errors.New("failed to parse PEM block containing the key")
		}

		if block.Type == "RSA PRIVATE KEY" {
			priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return nil, errors.Wrap(err, 0)
			}

			return priv, nil
		}
		pem_str = rest
	}
}

func ParseX509CertFromPemStr(pem_str []byte) (*x509.Certificate, error) {
	for {
		block, rest := pem.Decode(pem_str)
		if block == nil {
			return nil, errors.New("Unable to parse PEM")
		}

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, errors.Wrap(err, 0)
			}

			return cert, nil
		}
		pem_str = rest
	}
}

func ParseX509CSRFromPemStr(pem_str []byte) (*x509.CertificateRequest, error) {
	for {
		block, rest := pem.Decode(pem_str)
		if block == nil {
			return nil, errors.New("Unable to parse PEM")
		}

		if block.Type == "CERTIFICATE REQUEST" {
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			if err != nil {
				return nil, errors.Wrap(err, 0)
			}

			return csr, nil
		}
		pem_str = rest
	}
}

/*
	Velociraptor Client IDs are derived from the public key of the client.

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
		return nil, errors.Wrap(err, 0)
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
				return nil, errors.Wrap(err, 0)
			}

			return pub, nil
		}
		pem_str = rest
	}
}

// Verify the configuration, possibly updating default settings.
func VerifyConfig(config_obj *config_proto.Config) error {
	if config_obj.Client == nil || len(config_obj.Client.ServerUrls) == 0 {
		return errors.New("No server URLs configured!")
	}

	writeback_service := writeback.GetWritebackService()
	return writeback_service.MutateWriteback(config_obj,
		func(wb *config_proto.Writeback) error {
			if wb.PrivateKey != "" {
				// Add a client id for information here.
				if wb.ClientId == "" {
					private_key, err := ParseRsaPrivateKeyFromPemStr(
						[]byte(wb.PrivateKey))
					if err != nil {
						return errors.Wrap(err, 0)
					}

					wb.ClientId = ClientIDFromPublicKey(
						&private_key.PublicKey)
					return writeback.WritebackUpdateLevel1
				}

				return writeback.WritebackNoUpdate
			}

			fmt.Println("Generating new private key....")
			pem, err := GeneratePrivateKey()
			if err != nil {
				return errors.Wrap(err, 0)
			}

			private_key, err := ParseRsaPrivateKeyFromPemStr(pem)
			if err != nil {
				return errors.Wrap(err, 0)
			}

			// Add a client id for information here
			wb.ClientId = ClientIDFromPublicKey(&private_key.PublicKey)
			wb.PrivateKey = string(pem)

			return writeback.WritebackUpdateLevel1
		})
}

func GetSubjectName(cert *x509.Certificate) string {
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}

	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}

	if len(cert.IPAddresses) > 0 {
		return cert.IPAddresses[0].String()
	}

	return ""
}
