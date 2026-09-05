package packaging

import (
	"fmt"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *PackagingTestSuite) TestDEBClient() {
	spec := NewClientDebSpec()
	arch, err := getDebArch(self.elf_data)
	assert.NoError(self.T(), err)

	target_config, err := validateClientConfig(self.ConfigObj, min_client_config)
	assert.NoError(self.T(), err)

	self.sanitizeConfig(target_config)

	spec.SetRuntimeParameters(target_config, arch, "releaseX", "", 0, self.elf_data)

	builder, err := BuildDeb(spec)
	assert.NoError(self.T(), err)
	defer builder.Close()

	goldie.Assert(self.T(), "TestDEBClient",
		[]byte(builder.Debug()))
}

func (self *PackagingTestSuite) TestDEBClientWithServerConfig() {
	spec := NewClientDebSpec()
	arch, err := getDebArch(self.elf_data)
	assert.NoError(self.T(), err)

	target_config, err := validateClientConfig(
		self.ConfigObj, test_utils.SERVER_CONFIG)
	assert.NoError(self.T(), err)

	self.sanitizeConfig(target_config)

	spec.SetRuntimeParameters(target_config, arch, "releaseX", "", 0, self.elf_data)

	builder, err := BuildDeb(spec)
	assert.NoError(self.T(), err)
	defer builder.Close()

	// Client config stored in DEB should be stripped from all server-related fields.
	client_config_file, ok := builder.(*DEBBuilder).state.Get("/etc/velociraptor/client.config.yaml")
	assert.True(self.T(), ok)
	client_config_body := client_config_file.(string)
	assert.NotContains(self.T(), client_config_body, "Frontend:")
	assert.NotContains(self.T(), client_config_body, "Datastore:")
	assert.NotContains(self.T(), client_config_body, "GUI:")
}

// Invalid config as it is missing the Client part
func (self *PackagingTestSuite) TestDEBClientInvalidConfig() {
	_, err := validateClientConfig(self.ConfigObj,
		`
Frontend:
  server_urls:
    - http://localhost/
`)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(), "Invalid client config provided")
}

func (self *PackagingTestSuite) TestDEBServer() {
	spec := NewServerDebSpec()
	arch, err := getDebArch(self.elf_data)
	assert.NoError(self.T(), err)

	target_config, err := validateServerConfig(self.ConfigObj)
	assert.NoError(self.T(), err)
	spec.SetRuntimeParameters(target_config, arch, "releaseX", "", 0, self.elf_data)

	builder, err := BuildDeb(spec)
	assert.NoError(self.T(), err)
	defer builder.Close()

	goldie.Assert(self.T(), "TestDEBServer",
		[]byte(builder.Debug()))
}

func (self *PackagingTestSuite) TestDEBServerWithCustomUser() {
	spec := NewServerDebSpec()

	// As supplied by cmdline.
	server_user_val := "myexistinguser"
	server_group_val := ""

	spec.Expansion.ServiceUser = server_user_val
	spec.Expansion.ServiceGroup = server_group_val

	arch, err := getDebArch(self.elf_data)
	assert.NoError(self.T(), err)

	target_config, err := validateServerConfig(self.ConfigObj)
	assert.NoError(self.T(), err)
	spec.SetRuntimeParameters(target_config, arch, "releaseX", "", 0, self.elf_data)

	builder, err := BuildDeb(spec)
	assert.NoError(self.T(), err)
	defer builder.Close()

	service_file, ok := builder.(*DEBBuilder).state.Get(
		"/etc/systemd/system/velociraptor_server.service")
	assert.True(self.T(), ok)
	service_body := service_file.(string)
	assert.Contains(self.T(), service_body, fmt.Sprintf("User=%s", server_user_val))
	assert.Contains(self.T(), service_body, fmt.Sprintf("Group=%s", server_group_val))

	postin, ok := builder.(*DEBBuilder).state.Get("postinst")
	assert.True(self.T(), ok)
	postin_body := postin.(string)
	assert.Contains(self.T(), postin_body, fmt.Sprintf("getent group %s", server_group_val))
	assert.Contains(self.T(), postin_body, fmt.Sprintf("getent passwd %s", server_user_val))
	assert.NotContains(self.T(), postin_body, "getent passwd velociraptor")
}

func (self *PackagingTestSuite) TestDEBServerWithCustomUserAndGroup() {
	spec := NewServerDebSpec()

	// As supplied by cmdline.
	server_user_val := "myexistinguser"
	server_group_val := "myexistinggroup"
	spec.Expansion.ServiceUser = server_user_val
	spec.Expansion.ServiceGroup = server_group_val

	arch, err := getDebArch(self.elf_data)
	assert.NoError(self.T(), err)

	target_config, err := validateServerConfig(self.ConfigObj)
	assert.NoError(self.T(), err)
	spec.SetRuntimeParameters(target_config, arch, "releaseX", "", 0, self.elf_data)

	builder, err := BuildDeb(spec)
	assert.NoError(self.T(), err)
	defer builder.Close()

	service_file, ok := builder.(*DEBBuilder).state.Get(
		"/etc/systemd/system/velociraptor_server.service")
	assert.True(self.T(), ok)
	service_body := service_file.(string)
	assert.Contains(self.T(), service_body, fmt.Sprintf("User=%s", server_user_val))
	assert.Contains(self.T(), service_body, fmt.Sprintf("Group=%s", server_group_val))

	postin, ok := builder.(*DEBBuilder).state.Get("postinst")
	assert.True(self.T(), ok)
	postin_body := postin.(string)
	assert.Contains(self.T(), postin_body, fmt.Sprintf("getent group %s", server_group_val))
	assert.Contains(self.T(), postin_body, fmt.Sprintf("getent passwd %s", server_user_val))
	assert.Contains(self.T(), postin_body, fmt.Sprintf("--ingroup %s", server_group_val))
	assert.NotContains(self.T(), postin_body, "getent passwd velociraptor")
	assert.NotContains(self.T(), postin_body, "getent group velociraptor")
}

func (self *PackagingTestSuite) TestDEBServerMaster() {
	spec := NewServerDebSpec()
	arch, err := getDebArch(self.elf_data)
	assert.NoError(self.T(), err)

	self.ConfigObj.ExtraFrontends = []*config_proto.FrontendConfig{{
		Hostname: "www.example.com",
		BindPort: 8100,
	}}

	target_config, err := validateServerConfig(self.ConfigObj)
	assert.NoError(self.T(), err)
	spec.SetRuntimeParameters(target_config, arch, "releaseX", "master", 0, self.elf_data)

	builder, err := BuildDeb(spec)
	assert.NoError(self.T(), err)
	defer builder.Close()

	goldie.Assert(self.T(), "TestDEBServerMaster",
		[]byte(builder.Debug()))
}

func (self *PackagingTestSuite) TestDEBServerMinion() {
	spec := NewServerDebSpec()
	arch, err := getDebArch(self.elf_data)
	assert.NoError(self.T(), err)

	self.ConfigObj.ExtraFrontends = []*config_proto.FrontendConfig{{
		Hostname: "www.example.com",
		BindPort: 8100,
	}}

	target_config, err := validateServerConfig(self.ConfigObj)
	assert.NoError(self.T(), err)

	spec.SetRuntimeParameters(target_config, arch, "releaseX", "minion", 0, self.elf_data)

	builder, err := BuildDeb(spec)
	assert.NoError(self.T(), err)
	defer builder.Close()

	goldie.Assert(self.T(), "TestDEBServerMinion",
		[]byte(builder.Debug()))
}
