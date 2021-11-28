package search_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/search"
	"www.velocidex.com/golang/velociraptor/services/indexing"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

type TestSuite struct {
	test_utils.TestSuite

	clients []string
}

func (self *TestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	self.populatedClients()

	// Start the indexing service and wait for it to initialize.
	require.NoError(self.T(), self.Sm.Start(indexing.StartIndexingService))
}

// Make some clients in the index.
func (self *TestSuite) populatedClients() {
	self.clients = nil
	db, err := datastore.GetDB(self.ConfigObj)
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
				err := search.SetIndex(self.ConfigObj, client_id, client_id)
				assert.NoError(self.T(), err)

				path_manager := paths.NewClientPathManager(client_id)
				record := &actions_proto.ClientInfo{ClientId: client_id}
				err = db.SetSubject(self.ConfigObj, path_manager.Path(),
					record)
				assert.NoError(self.T(), err)
			}
		}
	}
}

func (self *TestSuite) TestEnumerateIndex() {
	// Read all clients.
	ctx := context.Background()
	searched_clients := []string{}
	for hit := range search.SearchIndexWithPrefix(
		ctx, self.ConfigObj, "", search.OPTION_ENTITY) {
		if hit != nil {
			client_id := hit.Entity
			searched_clients = append(searched_clients, client_id)
		}
	}

	assert.Equal(self.T(), self.clients, searched_clients)
}

func TestIndexing(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
