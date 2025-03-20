package acl_managers_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/sanity"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var (
	mock_definitions = []string{`
name: Server.Internal.UserManager
type: INTERNAL
`}
)

type TestSuite struct {
	test_utils.TestSuite
}

func (self *TestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.LoadArtifactsIntoConfig(mock_definitions)
	self.TestSuite.SetupTest()
}

func (self *TestSuite) TestLockdown() {
	users_manager := services.GetUserManager()

	// Create an admin user with administrator role.
	err := users_manager.SetUser(self.Ctx, &api_proto.VelociraptorUser{
		Name: "admin",
	})
	assert.NoError(self.T(), err)

	err = services.GrantRoles(self.ConfigObj, "admin", []string{"administrator"})
	assert.NoError(self.T(), err)

	acl_manager := acl_managers.NewServerACLManager(self.ConfigObj, "admin")

	// Check the user has COLLECT_CLIENT
	ok, err := acl_manager.CheckAccess(acls.COLLECT_CLIENT)
	assert.NoError(self.T(), err)
	assert.True(self.T(), ok)

	// Now simulate lockdown - first set the config file.
	self.ConfigObj.Lockdown = true

	// Now start the sanity checker because it will configure lock down.
	err = sanity.NewSanityCheckService(self.Ctx, nil, self.ConfigObj)
	assert.NoError(self.T(), err)

	// Checking again should reject it due to lockdown.
	ok, err = acl_manager.CheckAccess(acls.COLLECT_CLIENT)
	assert.ErrorContains(self.T(), err, "Server locked down")
	assert.False(self.T(), ok)
}

func TestServerACLManager(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
