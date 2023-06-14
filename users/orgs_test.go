package users_test

import (
	"github.com/Velocidex/ordereddict"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/users"
)

func (self *UserManagerTestSuite) TestListOrgs() {
	self.makeUsers()

	golden := ordereddict.NewDict()

	golden.Set("OrgAdmin Sees all orgs",
		users.GetOrgs(self.Ctx, "OrgAdmin"))

	golden.Set("AdminO1 Sees only O1 with nonce",
		users.GetOrgs(self.Ctx, "AdminO1"))

	golden.Set("UserO1 Sees only O1 but does not see nonce",
		users.GetOrgs(self.Ctx, "UserO1"))

	goldie.Assert(self.T(), "TestListOrgs", json.MustMarshalIndent(golden))
}
