package client_info_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
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

	self.LoadArtifacts([]string{`
name: Server.Internal.ClientPing
type: INTERNAL
`})
}

func (self *ClientInfoTestSuite) TestClientInfo() {
	// Fetch the client from the manager
	client_info_manager := services.GetClientInfoManager()
	client_info_manager.(*client_info.ClientInfoManager).Clock = self.clock

	info, err := client_info_manager.Get(self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), info.ClientId, self.client_id)
	assert.Equal(self.T(), info.Ping, uint64(0))

	// Update the IP address
	client_info_manager.UpdatePing(self.client_id, "127.0.0.1")

	// Now get the client record and check that it is updated
	info, err = client_info_manager.Get(self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), info.Ping, uint64(100*1000000))
	assert.Equal(self.T(), info.IpAddress, "127.0.0.1")

	// Now flush the record to storage
	client_info_manager.Flush(self.client_id)

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
	master_client_info_manager := services.GetClientInfoManager()
	master_client_info_manager.(*client_info.ClientInfoManager).Clock = self.clock

	// Create a minion client info manager.
	minion_client_info_manager := client_info.NewClientInfoManager(self.ConfigObj)
	minion_client_info_manager.Clock = self.clock

	err := minion_client_info_manager.Start(
		self.Sm.Ctx, self.ConfigObj, self.Sm.Wg)
	assert.NoError(self.T(), err)

	// Update the minion timestamp
	minion_client_info_manager.UpdatePing(self.client_id, "127.0.0.1")

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		client_info, err := master_client_info_manager.Get(self.client_id)
		assert.NoError(self.T(), err)
		return client_info.IpAddress == "127.0.0.1"
	})
}

func TestClientInfoService(t *testing.T) {
	suite.Run(t, &ClientInfoTestSuite{})
}
