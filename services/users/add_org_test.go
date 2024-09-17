package users_test

import (
	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *UserManagerTestSuite) TestAddUserToOrg() {
	self.makeUsers()

	golden := ordereddict.NewDict()

	admin_policy := &acl_proto.ApiClientACL{
		Roles: []string{"administrator"},
	}

	reader_policy := &acl_proto.ApiClientACL{
		Roles: []string{"reader"},
	}

	// Can a simple user add themselves to another org?
	users_manager := services.GetUserManager()
	err := users_manager.AddUserToOrg(
		self.Ctx, services.UseExistingUser,
		"UserO1", "UserO1", []string{"O2"}, admin_policy)
	assert.ErrorContains(self.T(), err, "PermissionDenied")

	// Can an admin in O1 just add a user to O2?
	err = users_manager.AddUserToOrg(
		self.Ctx, services.UseExistingUser,
		"AdminO1", "UserO1", []string{"O2"}, admin_policy)
	assert.ErrorContains(self.T(), err, "PermissionDenied")

	// Can an OrgAdmin add a user from O1 to O2?
	err = users_manager.AddUserToOrg(
		self.Ctx, services.UseExistingUser,
		"OrgAdmin", "AdminO1", []string{"O2"}, admin_policy)
	assert.NoError(self.T(), err)

	user_record, err := users_manager.GetUser(self.Ctx, "OrgAdmin", "AdminO1")
	assert.NoError(self.T(), err)

	golden.Set("AdminO1 belongs in O1 and O2", user_record)

	// Now AdminO1 is an admin in both O1 and O2 so they can add the
	// user there.
	err = users_manager.AddUserToOrg(
		self.Ctx, services.UseExistingUser,
		"AdminO1", "UserO1", []string{"O2"}, reader_policy)
	assert.NoError(self.T(), err)

	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO1")
	assert.NoError(self.T(), err)

	golden.Set("UserO1 belongs in O1 and O2", user_record)

	// Try to add an unknown user.
	err = users_manager.AddUserToOrg(
		self.Ctx, services.UseExistingUser,
		"OrgAdmin", "NoSuchUser", []string{"O2"}, admin_policy)
	assert.ErrorContains(self.T(), err, "User not found")

	// Request a new user record to be created.
	err = users_manager.AddUserToOrg(
		self.Ctx, services.AddNewUser,
		"AdminO2", "NoSuchUser", []string{"O2"}, reader_policy)
	assert.NoError(self.T(), err)

	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "NoSuchUser")
	assert.NoError(self.T(), err)
	golden.Set("New Users NoSuchUser", user_record)

	// Try to create a reserved user
	err = users_manager.AddUserToOrg(
		self.Ctx, services.AddNewUser,
		"AdminO2", utils.GetSuperuserName(self.ConfigObj),
		[]string{"O2"}, reader_policy)
	assert.ErrorContains(self.T(), err, "reserved")

	goldie.Assert(self.T(), "TestAddUserToOrg", json.MustMarshalIndent(golden))
}
