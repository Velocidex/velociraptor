package users_test

import (
	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *UserManagerTestSuite) TestDeleteUser() {
	self.makeUsers()

	golden := ordereddict.NewDict()

	// Can a user remove their own account? No but we just ignore
	// their request.
	err := users.DeleteUser(
		self.Ctx, "UserO1", "UserO1", []string{"O1"})
	assert.NoError(self.T(), err)

	user_record, err := users.GetUser(self.Ctx, "OrgAdmin", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("UserO1 delete UserO1", user_record)

	// Can AdminO1 remove a user in their org?
	err = users.DeleteUser(
		self.Ctx, "AdminO1", "UserO1", []string{"O1"})
	assert.NoError(self.T(), err)

	// Yes user is gone.
	user_record, err = users.GetUser(self.Ctx, "OrgAdmin", "UserO1")
	assert.ErrorContains(self.T(), err, "User not found")

	// UserO2 belongs in both O1 and O2
	reader_policy := &acl_proto.ApiClientACL{
		Roles: []string{"reader"},
	}

	err = users.AddUserToOrg(self.Ctx, users.UseExistingUser,
		"OrgAdmin", "UserO2", []string{"O1", "O2"}, reader_policy)
	assert.NoError(self.T(), err)

	user_record, err = users.GetUser(self.Ctx, "OrgAdmin", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("UserO2 is in O1 and O2", user_record)

	// AdminO2 will remove the user from all orgs, but they remain in
	// O1 because AdminO2 has no accesss to O1
	err = users.DeleteUser(
		self.Ctx, "AdminO2", "UserO2", users.LIST_ALL_ORGS)
	assert.NoError(self.T(), err)

	user_record, err = users.GetUser(self.Ctx, "OrgAdmin", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("UserO2 removed from O2", user_record)

	goldie.Assert(self.T(), "TestDeleteUser",
		json.MustMarshalIndent(golden))
}
