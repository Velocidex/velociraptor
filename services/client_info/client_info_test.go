package client_info_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/services/indexing"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type ClientInfoTestSuite struct {
	test_utils.TestSuite
	client_id string
}

func (self *ClientInfoTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	// For this test make the master write and sync quickly
	self.ConfigObj.Frontend.Resources.ClientInfoSyncTime = 1
	self.ConfigObj.Frontend.Resources.ClientInfoWriteTime = 1
	self.ConfigObj.Defaults.IndexedClientMetadata = []string{"dept"}

	self.LoadArtifactsIntoConfig([]string{`
name: Server.Internal.ClientPing
type: INTERNAL
`, `
name: Server.Internal.ClientInfoSnapshot
type: INTERNAL
`, `
name: Server.Internal.MetadataModifications
type: INTERNAL
`, `
name: Server.Audit.Logs
type: INTERNAL
`})

	// Create a client in the datastore so we can test initializing
	// client info manager from legacy datastore records.
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

	self.TestSuite.SetupTest()
}

func (self *ClientInfoTestSuite) TestClientInfoModify() {
	// Fetch the client from the manager
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	info, err := client_info_manager.Get(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), info.ClientId, self.client_id)
	assert.Equal(self.T(), info.Ping, uint64(0))

	// Update the ping time
	err = client_info_manager.Modify(self.Ctx, self.client_id,
		func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
			assert.NotNil(self.T(), client_info)

			client_info.Ping = 10
			return client_info, nil
		})
	assert.NoError(self.T(), err)

	// Now get the client record and check that it is updated
	info, err = client_info_manager.Get(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), info.Ping, uint64(10))

	// Now modify a nonexistant client - equivalent to a Set() call
	// (atomic check and set).
	err = client_info_manager.Modify(self.Ctx, "C.DOESNOTEXIT",
		func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
			assert.Nil(self.T(), client_info)
			return &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{
				ClientId: "C.DOESNOTEXIT",
				Ping:     20,
			}}, nil
		})
	assert.NoError(self.T(), err)

	info, err = client_info_manager.Get(self.Ctx, "C.DOESNOTEXIT")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), info.Ping, uint64(20))
}

func (self *ClientInfoTestSuite) TestClientInfo() {
	// Fetch the client from the manager
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Get a non-existing client id - should return an error
	_, err = client_info_manager.Get(self.Ctx, "C.DOESNOTEXIT")
	assert.Error(self.T(), err)

	info, err := client_info_manager.Get(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), info.ClientId, self.client_id)
	assert.Equal(self.T(), info.Ping, uint64(0))

	// Update the IP address
	client_info_manager.UpdateStats(self.Ctx,
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
}

// Check that master and minion update each other.
func (self *ClientInfoTestSuite) TestMasterMinion() {
	// Fetch the master client info manager
	master_client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Spin up a minion client_info manager
	minion_config := proto.Clone(self.ConfigObj).(*config_proto.Config)
	minion_config.Frontend.IsMinion = true
	minion_config.Frontend.Resources.MinionBatchWaitTimeMs = 1
	minion_config.Frontend.Resources.ClientInfoWriteTime = 1
	minion_config.Frontend.Resources.ClientInfoSyncTime = 1

	minion_client_info_manager, err := client_info.NewClientInfoManager(
		self.Ctx, self.Sm.Wg, minion_config)
	assert.NoError(self.T(), err)

	err = minion_client_info_manager.Start(
		self.Ctx, minion_config, self.Sm.Wg)
	assert.NoError(self.T(), err)

	// Update the minion timestamp
	minion_client_info_manager.UpdateStats(
		context.Background(), self.client_id, &services.Stats{
			IpAddress: "127.0.0.1",
		})

	// make sure the master node can see the update.
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		client_info, err := master_client_info_manager.Get(
			context.Background(), self.client_id)
		assert.NoError(self.T(), err)

		return client_info.IpAddress == "127.0.0.1"
	})
}

func (self *ClientInfoTestSuite) TestMetadataIndex() {
	sub_ctx, cancel := context.WithCancel(self.Ctx)
	defer cancel()

	wg := &sync.WaitGroup{}

	// Create our own client info manager so we can test shutdown and
	// restart.
	client_info_manager, err := client_info.NewClientInfoManager(
		sub_ctx, wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	// Add some metadata to the client record.
	err = client_info_manager.SetMetadata(self.Ctx, self.client_id,
		ordereddict.NewDict().
			Set("dept", "lawyers").Set("Field", "Value"), "admin")
	assert.NoError(self.T(), err)

	client_info_rec, err := client_info_manager.Get(sub_ctx, self.client_id)
	assert.NoError(self.T(), err)

	// Only the indexed fields will appear in the client info metadata
	assert.Equal(self.T(), 1, len(client_info_rec.Metadata))
	assert.Equal(self.T(), "lawyers", client_info_rec.Metadata["dept"])

	metadata, err := client_info_manager.GetMetadata(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 2, metadata.Len())
	field, _ := metadata.Get("Field")
	assert.Equal(self.T(), "Value", field)

	// Now ensure that we can search on the field
	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	response, err := indexer.SearchClients(self.Ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{
			Limit:    100,
			Query:    "dept:",
			NameOnly: true,
		}, "admin")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 1, len(response.Names))
	assert.Equal(self.T(), "dept:lawyers", response.Names[0])

	// This returns all matching records
	response, err = indexer.SearchClients(self.Ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{
			Limit: 100,
			Query: "dept:",
		}, "admin")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 1, len(response.Items))
	assert.Equal(self.T(), self.client_id, response.Items[0].ClientId)

	// Now update the metadata and ensure the search index is updated
	// to not return the old metadata hit and only return the new hit.
	err = client_info_manager.SetMetadata(self.Ctx, self.client_id,
		ordereddict.NewDict().Set("dept", "accountants"), "admin")
	assert.NoError(self.T(), err)

	response, err = indexer.SearchClients(self.Ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{
			Limit:    100,
			Query:    "dept:",
			NameOnly: true,
		}, "admin")
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 1, len(response.Names))
	assert.Equal(self.T(), "dept:accountants", response.Names[0])

	// Now close the client_info_manager and ensure the snapshot is
	// properly written.
	cancel()
	wg.Wait()

	new_client_info_manager, err := client_info.NewClientInfoManager(
		self.Ctx, self.Sm.Wg, self.ConfigObj)
	assert.NoError(self.T(), err)

	// Make sure the metadata was flushed to disk.
	metadata, err = new_client_info_manager.GetMetadata(
		self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), 2, metadata.Len())
	field, _ = metadata.Get("Field")
	assert.Equal(self.T(), "Value", field)

	// Get the global manager and force it to reload from storage.
	global_client_info_manager, err := services.GetClientInfoManager(
		self.ConfigObj)
	assert.NoError(self.T(), err)

	err = global_client_info_manager.(*client_info.ClientInfoManager).
		LoadFromSnapshot(self.Ctx, self.ConfigObj)
	assert.NoError(self.T(), err)

	// Make sure the indexer is populated with the metadata on
	// startup. It will query the global client info manager when it
	// rebuilds the index. This emulates an orderly startup.
	new_indexer, err := indexing.NewIndexingService(self.Ctx, self.Sm.Wg,
		self.ConfigObj)
	assert.NoError(self.T(), err)

	response, err = new_indexer.SearchClients(self.Ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{
			Limit:    100,
			Query:    "dept:",
			NameOnly: true,
		}, "admin")
	assert.NoError(self.T(), err)

	// We should be able to search for the metadata record.
	assert.Equal(self.T(), 1, len(response.Names))
	assert.Equal(self.T(), "dept:accountants", response.Names[0])
}

func TestClientInfoService(t *testing.T) {
	suite.Run(t, &ClientInfoTestSuite{})
}
