package collector_test

import (
	"path/filepath"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/filesystem"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
)

const (
	TestFrontendCertificate = `-----BEGIN CERTIFICATE-----
MIIDWTCCAkGgAwIBAgIQcyUFy1oMUr4O4sIOhom/jDANBgkqhkiG9w0BAQsFADAa
MRgwFgYDVQQKEw9WZWxvY2lyYXB0b3IgQ0EwIBcNMjMwNDEzMTgzMjUzWhgPMjEy
MzAzMjAxODMyNTNaMDQxFTATBgNVBAoTDFZlbG9jaXJhcHRvcjEbMBkGA1UEAxMS
VmVsb2NpcmFwdG9yU2VydmVyMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
AQEA9MSMbrFjmZs9bnpkel4vTQIyf+6Bpg60ByC7d6WWfBwvHdF1Qnfn1JO3Xo6p
53I1jPoagt0cZCzd6nwJXJ/3pclprmIOEBSc20pg5E0A/kpwn+bBoPNSrMF7+2/t
DvXP0Lvs/1OqUMjF8pCs6vnSKigaptn+0Et3GpzWjwCghqPcJBOuEuPQmR3HyHfs
dsMooCjuYcRcS9MXioT97SSjxeug0oTXHaKCnQ7txoxuN2+nNdr03mUu07TOUbRp
X3NsiaoESl/9IDC/tz2XTBD3UxLze9pX9t4tdKEMK2+gdnrnioOw1D7WBoElECj9
+89CRXlu3K15P1cNVB5htPzOgwIDAQABo38wfTAOBgNVHQ8BAf8EBAMCBaAwHQYD
VR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHwYDVR0j
BBgwFoAUO2IRSDwqgkZt5pkXdScs5BjoULEwHQYDVR0RBBYwFIISVmVsb2NpcmFw
dG9yU2VydmVyMA0GCSqGSIb3DQEBCwUAA4IBAQAhwcTMIdHqeR3FXOUREGjkjzC9
vz+hPdXB6w9CMYDOAsmQojuo09h84xt7jD0iqs/K1WJpLSNV3FG5C0TQXa3PD1l3
SsD5p4FfuqFACbPkm/oy+NA7E/0BZazC7iaZYjQw7a8FUx/P+eKo1S7z7Iq8HfmJ
yus5NlnoLmqb/3nZ7DyRWSo9HApmMdNjB6oJWrupSJajsw4Lsos2aJjkfzkg82W7
aGSh9S6Icn1f78BAjJVLv1QBNlb+yGOhrcUWQHERPEpkb1oZJwkVVE1XCZ1C4tVj
PtlBbpcpPHB/R5elxfo+We6vmC8+8XBlNPFFp8LAAile4uQPVQjqy7k/MZ4W
-----END CERTIFICATE-----`
	TestFrontendPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA9MSMbrFjmZs9bnpkel4vTQIyf+6Bpg60ByC7d6WWfBwvHdF1
Qnfn1JO3Xo6p53I1jPoagt0cZCzd6nwJXJ/3pclprmIOEBSc20pg5E0A/kpwn+bB
oPNSrMF7+2/tDvXP0Lvs/1OqUMjF8pCs6vnSKigaptn+0Et3GpzWjwCghqPcJBOu
EuPQmR3HyHfsdsMooCjuYcRcS9MXioT97SSjxeug0oTXHaKCnQ7txoxuN2+nNdr0
3mUu07TOUbRpX3NsiaoESl/9IDC/tz2XTBD3UxLze9pX9t4tdKEMK2+gdnrnioOw
1D7WBoElECj9+89CRXlu3K15P1cNVB5htPzOgwIDAQABAoIBAGAAy3gLOZ6hBgpU
FR7t3C2fRAFrogxozfHRw9Xc69ZIE67lXdGxSAvX2F9NI5T09c4Stt1HLoCYHH6B
Igbjc3XiNwI/0XY7L37PgItrLI2Q0vXUw3OGnJHH3gIz10472cPsQbuvrCi9Zu6K
ElijnewNCM8Sx+AZCWE1zO4P9+Z2kF9LvWzDwAa643jQ/Dg+S68zCFqjJCVJBGm+
LQxDs6dbArvOiEbuZs2wDt0d1kZF+BRljUTMoCpdf3jmFj3f0Jc1AFaz1eHG9Gte
XIUpbWmV2ATABSW2kDkVdXx+m/w1r9PZCLLfq54fIOlm2IeAiM3rDmM4ZSTUYEPn
mJP03xECgYEA+jS7DiS3bB/MeD+5qsgS07qJhOrX17s/SlamC1dQqz+koJLl98JX
CqyafFmdSz7PK2S2+OOazngwx26Kc3MZFoD9IQ2tuWmwDgbY8EQs5Cs37By2YRZJ
DdjvVf48pCKiXxIhvFjW/5CTemNAAu4CXg5Lkp7UVVrOmf5BmjMmE0sCgYEA+m+U
QMF0f7KLM4MU81yAMJdG4Sq4s9i4RmXes2FOUd4UoG7vEpycMKkmEaqiUVmRHPjp
P6Dwq3CK+FVFMpCeWjn6KkxwpdWWO9lglI0npFcPNW/PzPOv4mSNtCAcpHrKFP0R
3jbc8UhgtFxDZoeUih7cO2iTO7kELBCeKUzw9qkCgYBgVYcj1e0tWzztm5OP9sKQ
9MRYAdei/zxKEfySZ0bu+G0ZShXzA8dhm71LXXGbdA5t5bQxNej3z/zv/FagRtOE
/5r2a/7UYaXgcLB8KbOjEiTQ6ukpjlwIUdssn9uXUqJzulZ03zvAYFj4CVivCBav
Qg/E3xRf3LupPOTjSwhA6wKBgQDAH3tnlkHueSWLNiOLc0owfM12jhS2fCsabqpD
iQHRkoLWdWRZLeYw+oLnCLWPnRvTUy11j90yWJt0Wc5FNWcWJuZBLvU4c7vWXDRY
olVoIRXc09NiEwy6rJN9PSlcEYsYQPFFPWeQfwsZMrLOZHLS50vjE53oMk7+Ex2S
56DwSQKBgQC+iHbsbxloZjVMy01V21Sh9RwIpYrodEmwlTZf2jzaYloPadHu4MX1
jHG+zzeC/EJ3wFOKTSJ/Tmjo6N3Xaq9V7WeL8eBdtBtPztqN1yveTt94mZZ+fuID
BhI8P2RbNR2Yey5nnhFQcoTxpmVw3EYwE01nkxoPJRs/QVvxi9Mepg==
-----END RSA PRIVATE KEY-----`
)

type TestSuite struct {
	test_utils.TestSuite
}

func (self *TestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Frontend.Certificate = TestFrontendCertificate
	self.ConfigObj.Frontend.PrivateKey = TestFrontendPrivateKey

	self.TestSuite.SetupTest()
}

func (self *TestSuite) TestAutomaticDecryption() {
	manager, _ := services.GetRepositoryManager(self.ConfigObj)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	scope := manager.BuildScope(builder)

	fixture_path, _ := filepath.Abs(
		"../../vql/tools/collector/fixtures/offline_encrypted.zip")

	root_path_spec := (filesystem.PathSpecFunction{}).Call(self.Ctx, scope,
		ordereddict.NewDict().Set("DelegatePath", fixture_path))

	lines := []vfilter.Row{}
	for row := range (filesystem.GlobPlugin{}).Call(self.Ctx,
		scope, ordereddict.NewDict().
			Set("globs", "**").
			Set("accessor", "collector").
			Set("root", root_path_spec)) {

		full_path, _ := scope.Associative(row, "OSPath")
		full_path_path, _ := scope.Associative(full_path, "Path")
		lines = append(lines, full_path_path)
	}

	goldie.AssertJson(self.T(), "TestAutomaticDecryption", lines)
}

func TestCollectorAccessor(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
