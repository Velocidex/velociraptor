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
// Manage Velociraptor's CA and key signing.
package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto/utils"
)

type CertBundle struct {
	Cert       string
	PrivateKey string
}

func GenerateCACert(rsaBits int) (*CertBundle, error) {
	priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return nil, err
	}

	// Velociraptor depends on the CA certificate for
	// everything. It is embedded in clients and underpins
	// comms. We must ensure it does not expire in a reasonable
	// time.
	start_time := time.Now()
	end_time := start_time.Add(10 * 365 * 24 * time.Hour) // 10 years

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Velociraptor CA"},
		},
		NotBefore: start_time,
		NotAfter:  end_time,

		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature |
			x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
		DNSNames:              []string{"Velociraptor_ca.velocidex.com"},
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader, &template, &template,
		&priv.PublicKey,
		priv)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &CertBundle{
		PrivateKey: string(pem.EncodeToMemory(
			&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(priv),
			},
		)),
		Cert: string(pem.EncodeToMemory(
			&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: derBytes,
			},
		)),
	}, nil
}

func GenerateServerCert(config_obj *config_proto.Config, name string) (*CertBundle, error) {
	if config_obj.CA == nil {
		return nil, errors.New("No CA configured.")
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	start_time := time.Now()
	end_time := start_time.Add(365 * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}

	ca_cert, err := utils.ParseX509CertFromPemStr([]byte(
		config_obj.Client.CaCertificate))
	if err != nil {
		return nil, err
	}

	ca_private_key, err := utils.ParseRsaPrivateKeyFromPemStr(
		[]byte(config_obj.CA.PrivateKey))
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   name,
			Organization: []string{"Velociraptor"},
		},
		NotBefore: start_time,
		NotAfter:  end_time,

		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
	}

	// Encode the common name in the DNSNames field. Note that by
	// default Velociraptor pins the server name to
	// VelociraptorServer - it is not a DNS name at all. But since
	// golang 1.15 has deprecated the CommonName we need to use
	// this field or it will refuse to connect.
	// See https://github.com/golang/go/issues/39568#issuecomment-671424481
	ip := net.ParseIP(name)
	if ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	} else {
		template.DNSNames = append(template.DNSNames, name)
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader, &template, ca_cert,
		&priv.PublicKey,
		ca_private_key)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &CertBundle{
		PrivateKey: string(pem.EncodeToMemory(
			&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(priv),
			},
		)),
		Cert: string(pem.EncodeToMemory(
			&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: derBytes,
			},
		)),
	}, nil
}
