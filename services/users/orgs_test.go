package users_test

import (
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *UserManagerTestSuite) TestListOrgs() {
	self.makeUsers()

	golden := ordereddict.NewDict()
	users_manager := services.GetUserManager()
	golden.Set("OrgAdmin Sees all orgs",
		users_manager.GetOrgs(self.Ctx, "OrgAdmin"))

	golden.Set("AdminO1 Sees only O1 with nonce",
		users_manager.GetOrgs(self.Ctx, "AdminO1"))

	golden.Set("UserO1 Sees only O1 but does not see nonce",
		users_manager.GetOrgs(self.Ctx, "UserO1"))

	goldie.Assert(self.T(), "TestListOrgs", json.MustMarshalIndent(golden))
}
