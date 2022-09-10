package sanity_test

import (
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/services/sanity"
	"www.velocidex.com/golang/velociraptor/utils"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type ServicesTestSuite struct {
	test_utils.TestSuite
	client_id string
	flow_id   string
}

func (self *ServicesTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.LoadArtifacts([]string{`
name: TestArtifact
tools:
- name: Tool1
  url: https://www.example1.com/

- name: Tool2
  url: https://www.example2.com/

`})
	self.ConfigObj.Frontend.ServerServices.NotebookService = true
	self.ConfigObj.Frontend.ServerServices.UserManager = true

	self.TestSuite.SetupTest()
}

// Check tool upgrade.
func (self *ServicesTestSuite) TestUpgradeTools() {

	// Admin forces Tool1 to non-default
	inventory_service, err := services.GetInventory(self.ConfigObj)
	inventory_service.(*inventory.InventoryService).Clock = &utils.MockClock{MockNow: time.Unix(100, 0)}
	inventory_service.(*inventory.InventoryService).ClearForTests()

	tool_definition := &artifacts_proto.Tool{
		Name: "Tool1",
		Url:  "https://www.company.com",
	}
	err = inventory_service.AddTool(self.ConfigObj, tool_definition,
		services.ToolOptions{
			// This flag signifies that an admin explicitly set
			// this tool. We never overwrite an admin's setting.
			AdminOverride: true,
		})
	assert.NoError(self.T(), err)

	err = sanity.NewSanityCheckService(self.Ctx, self.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	db := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)
	inventory_config := &artifacts_proto.ThirdParty{}
	err = db.GetSubject(self.ConfigObj,
		paths.ThirdPartyInventory, inventory_config)
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict().Set("/config/inventory.json", inventory_config)

	serialized, err := json.MarshalIndentNormalized(golden)
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestUpgradeTools", serialized)
	// test_utils.GetMemoryDataStore(self.T(), self.config_obj).Debug()
}

// Make sure initial user is created.
func (self *ServicesTestSuite) TestCreateUser() {
	self.ConfigObj.GUI.Authenticator = &config_proto.Authenticator{Type: "Basic"}
	self.ConfigObj.GUI.InitialUsers = []*config_proto.GUIUser{
		{
			Name:         "User1",
			PasswordHash: "0d7dc4769a1d85162802703a1855b76e3b652bda3e0582ab32433f63dc6a0736",
			PasswordSalt: "0f61ad0fd6391513021242efb9ac780245cc21527fa3f9c5e552d47223e383a2",
		},
	}

	err := sanity.NewSanityCheckService(self.Ctx, self.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	db := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)

	user1 := &api_proto.VelociraptorUser{}
	user_path_manager := paths.NewUserPathManager("User1")
	err = db.GetSubject(self.ConfigObj, user_path_manager.Path(), user1)
	assert.NoError(self.T(), err)

	acl_obj := &acl_proto.ApiClientACL{}
	err = db.GetSubject(
		self.ConfigObj, user_path_manager.ACL(), acl_obj)
	assert.NoError(self.T(), err)

	golden := ordereddict.NewDict().
		Set("/users/User1", user1).
		Set("/acl/User1.json", acl_obj)

	serialized, err := json.MarshalIndentNormalized(golden)
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestCreateUser", serialized)
}

// Make sure initial orgs are created with initial user granted all orgs.
func (self *ServicesTestSuite) TestCreateUserInOrgs() {
	self.ConfigObj.GUI.Authenticator = &config_proto.Authenticator{Type: "Basic"}
	self.ConfigObj.GUI.InitialUsers = []*config_proto.GUIUser{
		{
			Name:         "User1",
			PasswordHash: "0d7dc4769a1d85162802703a1855b76e3b652bda3e0582ab32433f63dc6a0736",
			PasswordSalt: "0f61ad0fd6391513021242efb9ac780245cc21527fa3f9c5e552d47223e383a2",
		},
	}
	self.ConfigObj.GUI.InitialOrgs = []*config_proto.InitialOrgRecord{
		{Name: "Org1", OrgId: "O01"},

		// Second org has no org id - means it should get a new random
		// one.
		{Name: "Org2"},
	}

	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	// Mock the org id so it is not really random.
	org_manager.(*orgs.TestOrgManager).SetOrgIdForTesting("OT02")

	err = sanity.NewSanityCheckService(self.Ctx, self.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	db := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)

	// Gather the information about user accounts and orgs
	get_golden := func() []byte {
		user1 := &api_proto.VelociraptorUser{}
		user_path_manager := paths.NewUserPathManager("User1")
		err = db.GetSubject(self.ConfigObj, user_path_manager.Path(), user1)
		assert.NoError(self.T(), err)

		golden := ordereddict.NewDict().
			Set("/users/User1", user1)

		for _, org_record := range org_manager.ListOrgs() {
			// The nonce will be random each time so we eliminate it from
			// the golden image.
			assert.True(self.T(), org_record.Nonce != "")
			org_record.Nonce = "Nonce Of " + org_record.OrgId

			org_id := org_record.OrgId
			org_config_obj, err := org_manager.GetOrgConfig(org_id)
			assert.NoError(self.T(), err)

			acl_obj := &acl_proto.ApiClientACL{}
			err = db.GetSubject(org_config_obj, user_path_manager.ACL(), acl_obj)
			assert.NoError(self.T(), err)

			golden.Set(org_id+"/org", org_record).
				Set(org_id+"/acl/User1.json", acl_obj)
		}
		serialized, err := json.MarshalIndentNormalized(golden)
		assert.NoError(self.T(), err)

		return serialized
	}

	serialized := get_golden()
	goldie.Assert(self.T(), "TestCreateUserInOrgs", serialized)

	// Second run will not change anything since org and user creation
	// only happen on first run.
	err = sanity.NewSanityCheckService(self.Ctx, self.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	serialized = get_golden()
	goldie.Assert(self.T(), "TestCreateUserInOrgs", serialized)
}

func TestSanityService(t *testing.T) {
	suite.Run(t, &ServicesTestSuite{})
}
