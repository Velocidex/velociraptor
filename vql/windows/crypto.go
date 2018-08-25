package windows

// #cgo LDFLAGS: -lcrypt32
//
// int get_all_certs(void *context);
import "C"

import (
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"github.com/mattn/go-pointer"
	"strings"
	"unicode/utf16"
	"unsafe"
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

	// Make a copy of the slice so windows may free its own copy.
	der_cert := append(
		[]byte{},
		(*[1 << 30]byte)(unsafe.Pointer(cert))[0:len]...)

	certificates, err := x509.ParseCertificates(der_cert)
	if err != nil {
		return
	}

	store_name := append([]uint16{},
		(*[1 << 30]uint16)(unsafe.Pointer(store))[0:store_len]...)

	result := pointer.Restore(unsafe.Pointer(ctx)).(*certContext)
	for _, c := range certificates {
		cert_context := &CertContext{
			c, string(utf16.Decode(store_name))}

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

func runCertificates(scope *vfilter.Scope,
	args *vfilter.Dict) []vfilter.Row {
	var result []vfilter.Row

	// The context is passed to the cert walker.
	ctx := &certContext{}
	ptr := pointer.Save(ctx)
	defer pointer.Unref(ptr)

	C.get_all_certs(ptr)

	// Remove duplicates.
	seen := make(map[string]bool)
	for _, c := range ctx.Certs {
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
	})
}
