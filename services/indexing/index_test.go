package indexing_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type TestSuite struct {
	test_utils.TestSuite

	clients []string
}

func (self *TestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.IndexServer = true
	self.ConfigObj.Services.Label = true
	self.ConfigObj.Services.ClientInfo = true
	self.ConfigObj.Frontend.Resources.IndexSnapshotFrequency = 100000

	self.TestSuite.SetupTest()

	self.populatedClientRecords()
}

// Make some clients in the index.
func (self *TestSuite) populatedClientRecords() {
	self.clients = nil

	count := 0

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	bytes := []byte("00000000")
	for i := 0; i < 4; i++ {
		bytes[0] = byte(i)
		for k := 0; k < 4; k++ {
			bytes[3] = byte(k)
			for j := 0; j < 4; j++ {
				bytes[7] = byte(j)
				client_id := fmt.Sprintf("C.%02x", bytes)
				self.clients = append(self.clients, client_id)
				count++

				err := client_info_manager.Set(self.Ctx, &services.ClientInfo{
					&actions_proto.ClientInfo{
						ClientId: client_id,
					}})
				assert.NoError(self.T(), err)
			}
		}
	}

	labeler := services.GetLabeler(self.ConfigObj)
	err = labeler.SetClientLabel(
		self.Ctx, self.ConfigObj, "C.0030300030303002", "MyLabel")
	assert.NoError(self.T(), err)

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = indexer.RebuildIndex(self.Ctx, self.ConfigObj)
	assert.NoError(self.T(), err)
}

func (self *TestSuite) TestEnumerateIndex() {
	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Read all clients.
	ctx := context.Background()
	searched_clients := []string{}
	for hit := range indexer.SearchIndexWithPrefix(ctx, self.ConfigObj, "") {
		if hit != nil && hit.Term != "all" {
			client_id := hit.Entity
			searched_clients = append(searched_clients, client_id)
		}
	}

	assert.Equal(self.T(), len(self.clients), len(searched_clients))
	assert.Equal(self.T(), self.clients, searched_clients)
}

func TestIndexing(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
