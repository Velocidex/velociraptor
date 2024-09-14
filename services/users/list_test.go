package users_test

import (
	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *UserManagerTestSuite) TestListUsers() {
	self.makeUsers()

	golden := ordereddict.NewDict()

	// OrgAdmin is an org admin at the root org so should see all
	// users and all their orgs.
	users_manager := services.GetUserManager()
	user_list, err := users_manager.ListUsers(
		self.Ctx, "OrgAdmin", services.LIST_ALL_ORGS)
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin ListUsers", user_list)

	// Only list in one org
	user_list, err = users_manager.ListUsers(self.Ctx, "OrgAdmin", []string{"O1"})
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin ListUsers in O1", user_list)

	// AdminO1 can only see users in O1
	user_list, err = users_manager.ListUsers(
		self.Ctx, "AdminO1", services.LIST_ALL_ORGS)
	assert.NoError(self.T(), err)

	golden.Set("AdminO1 ListUsers", user_list)

	// UserO1 can only see user names in O1
	user_list, err = users_manager.ListUsers(
		self.Ctx, "UserO1", services.LIST_ALL_ORGS)
	assert.NoError(self.T(), err)

	golden.Set("UserO1 ListUsers", user_list)

	// Now add AdminO1 to both O1 and O2
	admin_policy := &acl_proto.ApiClientACL{
		Roles: []string{"administrator"},
	}
	err = users_manager.AddUserToOrg(self.Ctx, services.UseExistingUser,
		"OrgAdmin", "AdminO1", []string{"O1", "O2"}, admin_policy)
	assert.NoError(self.T(), err)

	// List users as AdminO2. AdminO2 can see AdminO1 in their org,
	// but the org list visible should only mention O2.
	user_list, err = users_manager.ListUsers(
		self.Ctx, "AdminO2", services.LIST_ALL_ORGS)
	assert.NoError(self.T(), err)
	golden.Set("AdminO2 ListUsers - Filtered AdminO1 Orgs", user_list)

	// But the actual record for AdminO1 still contains both
	user_list, err = users_manager.ListUsers(
		self.Ctx, "AdminO1", services.LIST_ALL_ORGS)
	assert.NoError(self.T(), err)
	golden.Set("AdminO1 ListUsers - Includes both AdminO1 Orgs", user_list)

	goldie.Assert(self.T(), "TestListUsers", json.MustMarshalIndent(golden))
}
