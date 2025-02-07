//go:build windows && cgo
// +build windows,cgo

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

package windows

// #cgo LDFLAGS: -lcrypt32
//
// int get_all_certs(void *context);
import "C"

import (
	"context"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"strings"
	"unicode/utf16"
	"unsafe"

	"github.com/Velocidex/ordereddict"
	"github.com/mattn/go-pointer"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

var (
	KeyUsageMap = map[x509.KeyUsage]string{
		x509.KeyUsageDigitalSignature:  "KeyUsageDigitalSignature",
		x509.KeyUsageContentCommitment: "KeyUsageContentCommitment",
		x509.KeyUsageKeyEncipherment:   "KeyUsageKeyEncipherment",
		x509.KeyUsageDataEncipherment:  "KeyUsageDataEncipherment",
		x509.KeyUsageKeyAgreement:      "KeyUsageKeyAgreement",
		x509.KeyUsageCertSign:          "KeyUsageCertSign",
		x509.KeyUsageCRLSign:           "KeyUsageCRLSign",
		x509.KeyUsageEncipherOnly:      "KeyUsageEncipherOnly",
		x509.KeyUsageDecipherOnly:      "KeyUsageDecipherOnly",
	}
)

//export cert_walker
func cert_walker(cert *C.char, len C.int,
	store *C.wchar_t, store_len C.int, ctx *C.int) {

	if len == 0 || len > 1<<12 {
		return
	}

	if store_len == 0 || store_len > 1<<12 {
		return
	}

	// This sometimes panics when the API returns crazy data.
	defer utils.CheckForPanic("cert %p, len %d", cert, len)

	// Make a copy of the slice so windows may free its own copy.
	der_cert := append(
		[]byte{},
		(*[1 << 12]byte)(unsafe.Pointer(cert))[0:len]...)

	certificates, err := x509.ParseCertificates(der_cert)
	if err != nil {
		return
	}

	store_name := append([]uint16{},
		(*[1 << 12]uint16)(unsafe.Pointer(store))[0:store_len]...)

	result := pointer.Restore(unsafe.Pointer(ctx)).(*certContext)
	for _, c := range certificates {
		if c == nil || c.SerialNumber == nil {
			continue
		}
		cert_context := &CertContext{
			Certificate: c,
			Store:       string(utf16.Decode(store_name))}

		result.Certs = append(result.Certs, cert_context)
	}
}

type certContext struct {
	Certs []*CertContext
}

type CertContext struct {
	*x509.Certificate
	Store string
}

func (self *CertContext) KeyUsageString() string {
	key_usage := self.KeyUsage
	result := []string{}
	for k, v := range KeyUsageMap {
		if k&key_usage > 0 {
			result = append(result, v)
		}
	}

	return strings.Join(result, ",")
}

func (self *CertContext) IsSelfSigned() (bool, error) {
	opts := x509.VerifyOptions{
		Roots: x509.NewCertPool(),
	}
	// A Self signed cert is one that verifies itself.
	opts.Roots.AddCert(self.Certificate)

	chain, err := self.Verify(opts)
	if err != nil {
		return false, err
	}

	return len(chain) > 0, nil
}

func (self *CertContext) SHA1() string {
	hash := sha1.Sum(self.Certificate.Raw)
	return hex.EncodeToString(hash[:])
}

func (self *CertContext) KeyStrength() int {
	public_key, ok := self.PublicKey.(*rsa.PublicKey)
	if ok {
		return public_key.N.BitLen()
	}

	return -1
}

func (self *CertContext) HexSerialNumber() string {
	return self.SerialNumber.Text(16)
}

func runCertificates(
	ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) []vfilter.Row {
	var result []vfilter.Row

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("certificates: %s", err)
		return result
	}

	// The context is passed to the cert walker.
	cert_ctx := &certContext{}
	ptr := pointer.Save(cert_ctx)
	defer pointer.Unref(ptr)

	C.get_all_certs(ptr)

	// Remove duplicates.
	seen := make(map[string]bool)
	for _, c := range cert_ctx.Certs {
		fp := c.SHA1()
		_, pres := seen[fp]
		if !pres {
			seen[fp] = true
			result = append(result, c)
		}
	}

	return result
}

func init() {
	vql_subsystem.RegisterPlugin(&vfilter.GenericListPlugin{
		PluginName: "certificates",
		Doc:        "Collect certificate from the system trust store.",
		Function:   runCertificates,
		Metadata:   vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	})
}
