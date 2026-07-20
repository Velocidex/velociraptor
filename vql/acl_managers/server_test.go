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

	// Clear the lockdown once we are done here.
	defer acls.SetLockdownToken(nil)

	// Checking again should reject it due to lockdown.
	ok, err = acl_manager.CheckAccess(acls.COLLECT_CLIENT)
	assert.ErrorContains(self.T(), err, "Server locked down")
	assert.False(self.T(), ok)
}

func (self *TestSuite) TestOrgAdmin() {
	users_manager := services.GetUserManager()

	// Create an admin user with administrator role.
	err := users_manager.SetUser(self.Ctx, &api_proto.VelociraptorUser{
		Name: "admin",
	})
	assert.NoError(self.T(), err)

	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	// Create a side org.
	org_record, err := org_manager.CreateNewOrg("SideOrg", "OXYZ", "1234")
	assert.NoError(self.T(), err)

	org_config_obj, err := org_manager.GetOrgConfig(org_record.Id)
	assert.NoError(self.T(), err)

	// Assign the user an ORG_ADMIN on the child org only.
	err = services.GrantRoles(org_config_obj, "admin",
		[]string{"administrator", "org_admin"})
	assert.NoError(self.T(), err)

	// Even though we set the org admin role on the child org, the
	// permission is missing because child orgs can not hold the org
	// admin permission.
	policy, err := services.GetEffectivePolicy(org_config_obj, "admin")
	assert.NoError(self.T(), err)
	assert.False(self.T(), policy.OrgAdmin)

	// Check permissions inside the org.
	acl_manager := acl_managers.NewServerACLManager(org_config_obj, "admin")

	// Check the user has COLLECT_CLIENT
	ok, err := acl_manager.CheckAccess(acls.SERVER_ADMIN)
	assert.NoError(self.T(), err)
	assert.True(self.T(), ok)

	// But the user should not have ORG_ADMIN because this permission
	// in only ever checked on the root org.
	_, err = acl_manager.CheckAccess(acls.ORG_ADMIN)
	assert.Error(self.T(), err)
	assert.ErrorContains(self.T(), err, "PermissionDenied")

	// Now let's give the user org admin in the root org.
	// Assign the user an ORG_ADMIN on the side org only.
	err = services.GrantRoles(self.ConfigObj, "admin", []string{"org_admin"})
	assert.NoError(self.T(), err)

	// Repeat the check in the child org, but this time it should pass.
	ok, err = acl_manager.CheckAccess(acls.ORG_ADMIN)
	assert.NoError(self.T(), err)
	assert.True(self.T(), ok)
}

func TestServerACLManager(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
