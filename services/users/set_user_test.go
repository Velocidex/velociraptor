package users_test

import (
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func (self *UserManagerTestSuite) TestSetUserPassword() {
	self.makeUsers()

	// Can a user update their password?
	users_manager := services.GetUserManager()
	err := users_manager.SetUserPassword(
		self.Ctx, self.ConfigObj, "UserO1", "UserO1", "MyPassword", "")
	assert.NoError(self.T(), err)

	// Verify the password
	ok, err := users_manager.VerifyPassword(
		self.Ctx, "UserO1", "UserO1", "MyPassword")
	assert.NoError(self.T(), err)
	assert.True(self.T(), ok)

	// Can a user update an admin's password?
	err = users_manager.SetUserPassword(
		self.Ctx, self.ConfigObj, "UserO1", "AdminO1", "MyPassword", "")
	assert.Error(self.T(), err, "PermissionDenied")

	// Can an admin update a user's password?
	err = users_manager.SetUserPassword(
		self.Ctx, self.ConfigObj, "AdminO1", "UserO1", "MyPassword", "")
	assert.NoError(self.T(), err)

	// Can a user set current org to a different org?
	err = users_manager.SetUserPassword(
		self.Ctx, self.ConfigObj, "UserO1", "UserO1", "", "O2")
	assert.Error(self.T(), err, "PermissionDenied")
}
