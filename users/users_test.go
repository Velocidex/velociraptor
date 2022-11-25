package users_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type UserManagerTestSuite struct {
	test_utils.TestSuite
}

func (self *UserManagerTestSuite) makeUserWithRoles(username, org_id, role string) {
	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	_, err = org_manager.GetOrgConfig(org_id)
	if err != nil {
		_, err = org_manager.CreateNewOrg(org_id, org_id)
		assert.NoError(self.T(), err)
	}

	user_manager := services.GetUserManager()
	err = user_manager.SetUser(self.Sm.Ctx,
		&api_proto.VelociraptorUser{
			Name: username,
			Orgs: []*api_proto.OrgRecord{
				{Id: org_id},
			},
		})
	assert.NoError(self.T(), err)

	// Grant the user admin access in their respective orgs.
	org_config, err := org_manager.GetOrgConfig(org_id)
	assert.NoError(self.T(), err)

	err = services.GrantRoles(org_config, username, []string{role})
	assert.NoError(self.T(), err)
}

func (self *UserManagerTestSuite) makeUsers() {
	self.makeUserWithRoles("OrgAdmin", "", "administrator")

	self.makeUserWithRoles("AdminO1", "O1", "administrator")
	self.makeUserWithRoles("UserO1", "O1", "reader")

	self.makeUserWithRoles("AdminO2", "O2", "administrator")
	self.makeUserWithRoles("UserO2", "O2", "reader")
}

func TestUserManger(t *testing.T) {
	suite.Run(t, &UserManagerTestSuite{})
}
