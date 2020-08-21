package sanity

import (
	"context"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ServicesTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	flow_id    string
	sm         *services.Service
}

func (self *ServicesTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))

	manager := services.GetRepositoryManager()
	manager.SetGlobalRepositoryForTests(manager.NewRepository())
}

func (self *ServicesTestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

// Check tool upgrade.
func (self *ServicesTestSuite) TestUpgradeTools() {
	repository, _ := services.GetRepositoryManager().GetGlobalRepository(self.config_obj)

	// An an artifact with two tools.
	repository.LoadYaml(`
name: TestArtifact
tools:
- name: Tool1
  url: https://www.example1.com/

- name: Tool2
  url: https://www.example2.com/

`, true)

	// Admin forces Tool1 to non-default
	inventory := services.GetInventory().(*inventory.InventoryService)
	inventory.Clock = utils.MockClock{MockNow: time.Unix(100, 0)}
	tool_definition := &artifacts_proto.Tool{
		Name: "Tool1",
		Url:  "https://www.company.com",

		// This flag signifies that an admin explicitly set
		// this tool. We never overwrite an admin's setting.
		AdminOverride: true,
	}
	err := inventory.AddTool(self.config_obj, tool_definition)
	assert.NoError(self.T(), err)

	require.NoError(self.T(), self.sm.Start(StartSanityCheckService))

	db := test_utils.GetMemoryDataStore(self.T(), self.config_obj)
	golden := ordereddict.NewDict().
		Set("/config/inventory.json", db.Subjects["/config/inventory.json"])

	goldie.Assert(self.T(), "TestUpgradeTools", json.MustMarshalIndent(golden))
	// test_utils.GetMemoryDataStore(self.T(), self.config_obj).Debug()
}

// Make sure initial user is created.
func (self *ServicesTestSuite) TestCreateUser() {
	self.config_obj.GUI.InitialUsers = []*config_proto.GUIUser{
		{
			Name:         "User1",
			PasswordHash: "0d7dc4769a1d85162802703a1855b76e3b652bda3e0582ab32433f63dc6a0736",
			PasswordSalt: "0f61ad0fd6391513021242efb9ac780245cc21527fa3f9c5e552d47223e383a2",
		},
	}
	require.NoError(self.T(), self.sm.Start(StartSanityCheckService))

	db := test_utils.GetMemoryDataStore(self.T(), self.config_obj)
	golden := ordereddict.NewDict().
		Set("/users/User1", db.Subjects["/users/User1"]).
		Set("/acl/User1.json", db.Subjects["/acl/User1.json"])

	goldie.Assert(self.T(), "TestCreateUser", json.MustMarshalIndent(golden))
	// test_utils.GetMemoryDataStore(self.T(), self.config_obj).Debug()
}

func TestSanityService(t *testing.T) {
	suite.Run(t, &ServicesTestSuite{})
}
