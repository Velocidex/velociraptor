package client_info_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type ClientInfoTestSuite struct {
	test_utils.TestSuite
	client_id string
	clock     *utils.MockClock
}

func (self *ClientInfoTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	// For this test make the master write and sync quickly
	self.ConfigObj.Frontend.Resources.ClientInfoSyncTime = 1
	self.ConfigObj.Frontend.Resources.ClientInfoWriteTime = 1

	self.LoadArtifacts([]string{`
name: Server.Internal.ClientPing
type: INTERNAL
`})

	self.TestSuite.SetupTest()

	// Create a client in the datastore
	self.client_id = "C.1234"
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	client_info := &actions_proto.ClientInfo{
		ClientId: self.client_id,
		Hostname: "Hostname",
	}
	err = db.SetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
	assert.NoError(self.T(), err)

	self.clock = &utils.MockClock{
		MockNow: time.Unix(100, 0),
	}
}

func (self *ClientInfoTestSuite) TestClientInfo() {
	// Fetch the client from the manager
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_info_manager.(*client_info.ClientInfoManager).Clock = self.clock

	// Get a non-existing client id - should return an error
	_, err = client_info_manager.Get(context.Background(), "C.DOESNOTEXIT")
	assert.Error(self.T(), err)

	info, err := client_info_manager.Get(context.Background(), self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), info.ClientId, self.client_id)
	assert.Equal(self.T(), info.Ping, uint64(0))

	// Update the IP address
	client_info_manager.UpdateStats(context.Background(),
		self.client_id, &services.Stats{
			Ping:      uint64(100 * 1000000),
			IpAddress: "127.0.0.1",
		})

	// Now get the client record and check that it is updated
	info, err = client_info_manager.Get(
		context.Background(), self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), info.Ping, uint64(100*1000000))
	assert.Equal(self.T(), info.IpAddress, "127.0.0.1")

	// Now flush the record to storage
	client_info_manager.Flush(context.Background(), self.client_id)

	// Check the stored ping record
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	stored_client_info := &actions_proto.ClientInfo{}
	err = db.GetSubject(self.ConfigObj, client_path_manager.Ping(),
		stored_client_info)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), stored_client_info.IpAddress, "127.0.0.1")
}

// Check that master and minion update each other.
func (self *ClientInfoTestSuite) TestMasterMinion() {
	// Fetch the master client info manager
	master_client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)
	master_client_info_manager.(*client_info.ClientInfoManager).Clock = self.clock

	// Spin up a minion client_info manager
	minion_config := proto.Clone(self.ConfigObj).(*config_proto.Config)
	minion_config.Frontend.IsMinion = true
	minion_config.Frontend.Resources.MinionBatchWaitTimeMs = 1
	minion_config.Frontend.Resources.ClientInfoWriteTime = 1
	minion_config.Frontend.Resources.ClientInfoSyncTime = 1

	minion_client_info_manager := client_info.NewClientInfoManager(minion_config)
	minion_client_info_manager.Clock = self.clock

	err = minion_client_info_manager.Start(
		self.Sm.Ctx, minion_config, self.Sm.Wg)
	assert.NoError(self.T(), err)

	// Update the minion timestamp
	minion_client_info_manager.UpdateStats(
		context.Background(), self.client_id, &services.Stats{
			IpAddress: "127.0.0.1",
		})

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		client_info, err := master_client_info_manager.Get(
			context.Background(), self.client_id)
		assert.NoError(self.T(), err)
		return client_info.IpAddress == "127.0.0.1"
	})

	// Make sure we actually write the result in the datastore.
	client_path_manager := paths.NewClientPathManager(self.client_id)

	// Read the main client record
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		ping_info := &services.ClientInfo{}
		db.GetSubject(self.ConfigObj, client_path_manager.Ping(),
			ping_info)
		utils.Debug(ping_info)
		return ping_info.IpAddress == "127.0.0.1"
	})
}

func TestClientInfoService(t *testing.T) {
	suite.Run(t, &ClientInfoTestSuite{})
}
