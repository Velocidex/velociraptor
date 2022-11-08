package users_test

import (
	"www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *UserManagerTestSuite) TestSetUserPassword() {
	self.makeUsers()

	// Can a user update their password?
	err := users.SetUserPassword(
		self.Ctx, "UserO1", "UserO1", "MyPassword", "")
	assert.NoError(self.T(), err)

	// Verify the password
	ok, err := users.VerifyPassword(
		self.Ctx, "UserO1", "UserO1", "MyPassword")
	assert.NoError(self.T(), err)
	assert.True(self.T(), ok)

	// Can a user update an admin's password?
	err = users.SetUserPassword(
		self.Ctx, "UserO1", "AdminO1", "MyPassword", "")
	assert.Error(self.T(), err, "PermissionDenied")

	// Can an admin update a user's password?
	err = users.SetUserPassword(
		self.Ctx, "AdminO1", "UserO1", "MyPassword", "")
	assert.NoError(self.T(), err)

	// Can a user set current org to a different org?
	err = users.SetUserPassword(
		self.Ctx, "UserO1", "UserO1", "", "O2")
	assert.Error(self.T(), err, "PermissionDenied")
}
