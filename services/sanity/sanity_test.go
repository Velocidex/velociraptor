package sanity

import (
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/require"
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
	"www.velocidex.com/golang/velociraptor/utils"
)

type ServicesTestSuite struct {
	test_utils.TestSuite
	client_id string
	flow_id   string
}

// Check tool upgrade.
func (self *ServicesTestSuite) TestUpgradeTools() {
	self.LoadArtifacts([]string{`
name: TestArtifact
tools:
- name: Tool1
  url: https://www.example1.com/

- name: Tool2
  url: https://www.example2.com/

`})

	// Admin forces Tool1 to non-default
	inventory := services.GetInventory().(*inventory.InventoryService)
	inventory.Clock = &utils.MockClock{MockNow: time.Unix(100, 0)}
	inventory.ClearForTests()

	tool_definition := &artifacts_proto.Tool{
		Name: "Tool1",
		Url:  "https://www.company.com",
	}
	err := inventory.AddTool(self.ConfigObj, tool_definition,
		services.ToolOptions{
			// This flag signifies that an admin explicitly set
			// this tool. We never overwrite an admin's setting.
			AdminOverride: true,
		})
	assert.NoError(self.T(), err)

	require.NoError(self.T(), self.Sm.Start(StartSanityCheckService))

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
	require.NoError(self.T(), self.Sm.Start(StartSanityCheckService))

	db := test_utils.GetMemoryDataStore(self.T(), self.ConfigObj)

	user1 := &api_proto.VelociraptorUser{}
	user_path_manager := paths.NewUserPathManager("User1")
	err := db.GetSubject(self.ConfigObj, user_path_manager.Path(), user1)
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

func TestSanityService(t *testing.T) {
	suite.Run(t, &ServicesTestSuite{})
}
