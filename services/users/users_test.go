package users_test

import (
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type UserManagerTestSuite struct {
	test_utils.TestSuite
}

func (self *UserManagerTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.JournalService = true

	self.TestSuite.SetupTest()

	self.LoadArtifacts(`name: Server.Audit.Logs
type: SERVER_EVENT
`, `name: Server.Internal.UserManager
type: INTERNAL
`)
}

func (self *UserManagerTestSuite) makeUserWithRoles(username, org_id, role string) {
	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	_, err = org_manager.GetOrgConfig(org_id)
	if err != nil {
		_, err = org_manager.CreateNewOrg(org_id, org_id, services.RandomNonce)
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

// The rest of the tests depend on this state being correct.  Make sure it is.
func (self *UserManagerTestSuite) TestMakeUsers() {
	self.makeUsers()

	golden := ordereddict.NewDict()
	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUser(self.Ctx, "OrgAdmin", "OrgAdmin")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin OrgAdmin", user_record)

	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin UserO1", user_record)

	user_record, err = users_manager.GetUser(self.Ctx, "AdminO1", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("AdminO1 UserO1", user_record)

	user_record, err = users_manager.GetUser(self.Ctx, "AdminO2", "UserO1")
	assert.ErrorContains(self.T(), err, "PermissionDenied")
	golden.Set("AdminO2 UserO1", err.Error())

	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin UserO2", user_record)

	user_record, err = users_manager.GetUser(self.Ctx, "AdminO2", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("AdminO2 UserO2", user_record)

	user_record, err = users_manager.GetUser(self.Ctx, "AdminO1", "UserO2")
	assert.ErrorContains(self.T(), err, "PermissionDenied")
	golden.Set("AdminO1 UserO2", err.Error())

	// Check case insensitive user records
	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "userO2")
	assert.NoError(self.T(), err)
	golden.Set("Case insensitive user02", user_record)

	goldie.Assert(self.T(), "TestMakeUsers", json.MustMarshalIndent(golden))
}

func filterUser(users []*api_proto.VelociraptorUser, username string) (
	res []*api_proto.VelociraptorUser) {
	for _, i := range users {
		if strings.EqualFold(i.Name, username) {
			res = append(res, i)
		}
	}
	return res
}

func TestUserManger(t *testing.T) {
	orgs.NonceForTest = "Nonce"

	suite.Run(t, &UserManagerTestSuite{})
}
