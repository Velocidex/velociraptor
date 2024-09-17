package indexing_test

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func (self *TestSuite) TestWildCardSearch() {
	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Read all clients.
	results := ordereddict.NewDict()
	for _, search_term := range []string{
		"client:C.023030003030*2",
		"client:*.023030003030*2",
		"client:*30003030*2",
		"client:C.02303*2",
	} {
		ctx := context.Background()
		scope := vql_subsystem.MakeScope()
		searched_clients := []string{}
		search_chan, err := indexer.SearchClientsChan(
			ctx, scope, self.ConfigObj, search_term, "")
		assert.NoError(self.T(), err)

		for hit := range search_chan {
			if hit != nil {
				searched_clients = append(searched_clients, hit.ClientId)
			}
		}
		results.Set(search_term, searched_clients)
	}

	goldie.Assert(self.T(), "TestWildCardSearch",
		json.MustMarshalIndent(results))
}

func (self *TestSuite) TestPrefixSearch() {
	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Read all clients.
	prefix := "C.0230300330"
	ctx := context.Background()
	searched_clients := []string{}
	for hit := range indexer.SearchIndexWithPrefix(ctx, self.ConfigObj, prefix) {
		if hit != nil {
			client_id := hit.Entity
			searched_clients = append(searched_clients, client_id)
		}
	}

	prefixed_clients := []string{}
	for _, client_id := range self.clients {
		if strings.HasPrefix(client_id, prefix) {
			prefixed_clients = append(prefixed_clients, client_id)
		}
	}
	assert.Equal(self.T(), prefixed_clients, searched_clients)
}
