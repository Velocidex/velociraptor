package packaging

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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

	goldie.Assert(self.T(), "TestDEBClient",
		[]byte(builder.Debug()))
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

	goldie.Assert(self.T(), "TestDEBServer",
		[]byte(builder.Debug()))
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

	goldie.Assert(self.T(), "TestDEBServerMinion",
		[]byte(builder.Debug()))
}
