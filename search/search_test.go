package search_test

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/search"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func (self *TestSuite) TestWildCardSearch() {
	self.populatedClients()

	// Read all clients.
	results := ordereddict.NewDict()
	for _, search_term := range []string{
		"C.023030003030*2",
		"*.023030003030*2",
		"*30003030*2",
		"C.02303*2",
	} {
		ctx := context.Background()
		scope := vql_subsystem.MakeScope()
		searched_clients := []string{}
		search_chan, err := search.SearchClientsChan(
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
	self.populatedClients()

	initial_op_count := getIndexListings(self.T())

	// Read all clients.
	prefix := "C.0230300330"
	ctx := context.Background()
	searched_clients := []string{}
	for hit := range search.SearchIndexWithPrefix(
		ctx, self.ConfigObj, prefix, search.OPTION_ENTITY) {
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

	current_op_count := getIndexListings(self.T())
	assert.Equal(self.T(), uint64(34), current_op_count-initial_op_count)
}
