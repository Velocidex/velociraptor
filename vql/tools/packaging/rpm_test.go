package packaging

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/google/rpmpack"
	"github.com/stretchr/testify/suite"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

const (
	min_client_config = `
Client:
  server_urls:
    - http://localhost/
  nonce: XXX
  ca_certificate: |
    -----BEGIN CERTIFICATE-----
    MIIDTDCCAjSgAwIBAgIRAJH2OrT69FpC7IT3ZeZLmXgwDQYJKoZIhvcNAQELBQAw
    -----END CERTIFICATE-----
`
)

type PackagingTestSuite struct {
	test_utils.TestSuite
	elf_data []byte
}

func (self *PackagingTestSuite) sanitizeConfig(config_obj *config_proto.Config) {
	// Remove the version so we can use the golden fixtures
	config_obj.Version = &config_proto.Version{
		Version: "0.74.3",
	}
	if config_obj.Client != nil {
		config_obj.Client.Version = self.ConfigObj.Version
		config_obj.Client.ServerVersion = self.ConfigObj.Version
	}
	config_obj.Autoexec = nil
}

func (self *PackagingTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	self.sanitizeConfig(self.ConfigObj)

	fd, err := os.Open("../../../artifacts/testdata/files/test.elf")
	assert.NoError(self.T(), err)
	self.elf_data, err = ioutil.ReadAll(fd)
	assert.NoError(self.T(), err)
}

func (self *PackagingTestSuite) TestRPMClient() {
	spec := NewClientRPMSpec()
	target_config, err := validateClientConfig(self.ConfigObj, min_client_config)
	assert.NoError(self.T(), err)

	self.sanitizeConfig(target_config)

	arch, err := getRPMArch(self.elf_data)
	assert.NoError(self.T(), err)
	spec.SetRuntimeParameters(
		target_config, arch, "releaseX", "client", 0, self.elf_data)

	builder, err := BuildRPM(spec)
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestRPMClient",
		[]byte(builder.Debug()))
}

func (self *PackagingTestSuite) TestRPMClientWithServerConfig() {
	spec := NewClientRPMSpec()
	target_config, err := validateClientConfig(
		self.ConfigObj, test_utils.SERVER_CONFIG)
	assert.NoError(self.T(), err)

	self.sanitizeConfig(target_config)

	arch, err := getRPMArch(self.elf_data)
	assert.NoError(self.T(), err)

	spec.SetRuntimeParameters(
		target_config, arch, "releaseX", "client", 0, self.elf_data)

	builder, err := BuildRPM(spec)
	assert.NoError(self.T(), err)

	// Client config stored in RPM should be stripped from all server
	// related fields.
	client_config_file, _ := builder.(*RPMBuilder).state.Get("/etc/velociraptor/client.config.yaml")
	goldie.Assert(self.T(), "TestRPMClientWithServerConfig",
		client_config_file.(rpmpack.RPMFile).Body)
}

// Invalid config as it is missing the Client part
func (self *PackagingTestSuite) TestRPMClientInvalidConfig() {
	_, err := validateClientConfig(self.ConfigObj,
		`
Frontend:
  server_urls:
    - http://localhost/
`)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "Invalid client config provided")
}

func (self *PackagingTestSuite) TestRPMServer() {
	spec := NewServerRPMSpec()
	arch, err := getRPMArch(self.elf_data)
	assert.NoError(self.T(), err)

	target_config, err := validateServerConfig(self.ConfigObj)
	assert.NoError(self.T(), err)

	spec.SetRuntimeParameters(
		target_config, arch, "releaseX", "server", 0, self.elf_data)

	builder, err := BuildRPM(spec)
	assert.NoError(self.T(), err)

	fixture := []byte(builder.Debug())
	config_file, _ := builder.(*RPMBuilder).state.Get("/etc/velociraptor/server.config.yaml")
	fixture = append(fixture, []byte("/etc/velociraptor/server.config.yaml\n-----\n")...)
	fixture = append(fixture, config_file.(rpmpack.RPMFile).Body...)

	goldie.Assert(self.T(), "TestRPMServer", fixture)
}

func (self *PackagingTestSuite) TestRPMServerMaster() {
	spec := NewServerRPMSpec()
	arch, err := getRPMArch(self.elf_data)
	assert.NoError(self.T(), err)

	self.ConfigObj.ExtraFrontends = []*config_proto.FrontendConfig{{
		Hostname: "www.example.com",
		BindPort: 8100,
	}}

	target_config, err := validateServerConfig(self.ConfigObj)
	assert.NoError(self.T(), err)

	spec.SetRuntimeParameters(
		target_config, arch, "releaseX", "master", 0, self.elf_data)

	builder, err := BuildRPM(spec)
	assert.NoError(self.T(), err)

	fixture := []byte(builder.Debug())
	config_file, _ := builder.(*RPMBuilder).state.Get(
		"/etc/systemd/system/velociraptor_server.service")
	fixture = append(fixture, []byte("/etc/systemd/system/velociraptor_server.service\n-----\n")...)
	fixture = append(fixture, config_file.(rpmpack.RPMFile).Body...)

	goldie.Assert(self.T(), "TestRPMServerMaster", fixture)
}

func (self *PackagingTestSuite) TestRPMServerMinion() {
	spec := NewServerRPMSpec()
	arch, err := getRPMArch(self.elf_data)
	assert.NoError(self.T(), err)

	self.ConfigObj.ExtraFrontends = []*config_proto.FrontendConfig{{
		Hostname: "www.example.com",
		BindPort: 8100,
	}}

	target_config, err := validateServerConfig(self.ConfigObj)
	assert.NoError(self.T(), err)

	spec.SetRuntimeParameters(
		target_config, arch, "releaseX", "minion", 0, self.elf_data)

	builder, err := BuildRPM(spec)
	assert.NoError(self.T(), err)

	fixture := []byte(builder.Debug())
	config_file, _ := builder.(*RPMBuilder).state.Get(
		"/etc/systemd/system/velociraptor_server.service")
	fixture = append(fixture, []byte("/etc/systemd/system/velociraptor_server.service\n-----\n")...)
	fixture = append(fixture, config_file.(rpmpack.RPMFile).Body...)

	goldie.Assert(self.T(), "TestRPMServerMinion", fixture)
}

func TestPackaging(t *testing.T) {
	suite.Run(t, &PackagingTestSuite{})
}
