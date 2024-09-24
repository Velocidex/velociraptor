package users_test

import (
	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *UserManagerTestSuite) TestGetUsers() {
	self.makeUsers()

	golden := ordereddict.NewDict()

	// Can a simple user get their own record? Should get the full record.
	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUser(self.Ctx, "UserO1", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("UserO1 GetUser UserO1", user_record)

	// Can they get a different user in their own orgs? Only the name!
	user_record, err = users_manager.GetUser(self.Ctx, "UserO1", "AdminO1")
	assert.NoError(self.T(), err)

	golden.Set("UserO1 GetUser AdminO1", user_record)

	// Can they get a different user in another org? Nope.
	user_record, err = users_manager.GetUser(self.Ctx, "UserO1", "UserO2")
	assert.Error(self.T(), err, "PermissionDenied")

	// An admin can get any user in their org - full record
	user_record, err = users_manager.GetUser(self.Ctx, "AdminO1", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("AdminO1 GetUser UserO1", user_record)

	// But an admin in one org can not see users in another org
	user_record, err = users_manager.GetUser(self.Ctx, "AdminO1", "AdminO2")
	assert.Error(self.T(), err, "PermissionDenied")

	// Getting an invalid user gives PermissionDenied
	user_record, err = users_manager.GetUser(self.Ctx, "AdminO1", "InvalidUsername")
	assert.Error(self.T(), err, "PermissionDenied")

	// An org admin can see all users
	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin GetUser UserO1", user_record)

	// Now add AdminO1 and UserO1 to both O1 and O2
	admin_policy := &acl_proto.ApiClientACL{
		Roles: []string{"administrator"},
	}
	err = users_manager.AddUserToOrg(self.Ctx, services.UseExistingUser,
		"OrgAdmin", "AdminO1", []string{"O1", "O2"}, admin_policy)
	assert.NoError(self.T(), err)

	err = users_manager.AddUserToOrg(self.Ctx, services.UseExistingUser,
		"OrgAdmin", "UserO1", []string{"O1", "O2"}, admin_policy)
	assert.NoError(self.T(), err)

	// When AdminO2 looks at UserO1 they can only see the O2
	// membership because AdminO2 does not have access to O1.
	user_record, err = users_manager.GetUser(self.Ctx, "AdminO2", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("AdminO2 GetUser UserO1 - filtered Org list", user_record)

	// When AdminO1 looks at UserO1 they can see all the Org
	// memberships because AdminO1 does have access to O1 and O2.
	user_record, err = users_manager.GetUser(self.Ctx, "AdminO1", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("AdminO1 GetUser UserO1 - can see full Org list", user_record)

	goldie.Assert(self.T(), "TestGetUsers", json.MustMarshalIndent(golden))
}
