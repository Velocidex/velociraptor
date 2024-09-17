package users_test

import (
	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *UserManagerTestSuite) TestDeleteUser() {
	self.makeUsers()

	golden := ordereddict.NewDict()

	// Can a user remove their own account? No but we just ignore
	// their request.
	users_manager := services.GetUserManager()
	err := users_manager.DeleteUser(
		self.Ctx, "UserO1", "UserO1", []string{"O1"})
	assert.NoError(self.T(), err)

	user_record, err := users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin GetUser UserO1", user_record)

	// Can AdminO1 remove a user in their org?
	err = users_manager.DeleteUser(
		self.Ctx, "AdminO1", "UserO1", []string{"O1"})
	assert.NoError(self.T(), err)

	// Yes user is gone.
	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO1")
	assert.ErrorContains(self.T(), err, "User not found")

	// UserO2 belongs in both O1 and O2
	reader_policy := &acl_proto.ApiClientACL{
		Roles: []string{"reader"},
	}

	err = users_manager.AddUserToOrg(self.Ctx, services.UseExistingUser,
		"OrgAdmin", "UserO2", []string{"O1", "O2"}, reader_policy)
	assert.NoError(self.T(), err)

	// Lookup using ORG_ADMIN
	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin UserO2 is in O1 and O2", user_record)

	// Lookup using O1's SERVER_ADMIN
	user_record, err = users_manager.GetUser(self.Ctx, "AdminO1", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("AdminO1 UserO2 is in O1", user_record)

	// Lookup using O2's SERVER_ADMIN
	user_record, err = users_manager.GetUser(self.Ctx, "AdminO2", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("AdminO2 UserO2 is in O2", user_record)

	// AdminO2 will remove the user from all orgs, but they remain in
	// O1 because AdminO2 has no accesss to O1
	err = users_manager.DeleteUser(
		self.Ctx, "AdminO2", "UserO2", services.LIST_ALL_ORGS)
	assert.NoError(self.T(), err)

	// GetUser returns PermissionDenied if the user requesting does
	// not have OrgAdmin and does not belong to any of the same orgs
	user_record, err = users_manager.GetUser(self.Ctx, "AdminO2", "UserO2")
	assert.ErrorContains(self.T(), err, "PermissionDenied")
	golden.Set("AdminO2 UserO2 removed from O2", err.Error())

	// test_utils.GetMemoryDataStore(self.T(), self.ConfigObj).Debug(self.ConfigObj)

	// If the user was added to O1 and removed from O2, it should
	// still exist in O1
	user_record, err = users_manager.GetUser(self.Ctx, "AdminO1", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("AdminO1 UserO2 still in O1", user_record)

	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin UserO2 removed from O2", user_record)

	goldie.Assert(self.T(), "TestDeleteUser",
		json.MustMarshalIndent(golden))
}
