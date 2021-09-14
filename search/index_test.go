package search_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alecthomas/assert"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/search"
	"www.velocidex.com/golang/velociraptor/utils"
)

type TestSuite struct {
	test_utils.TestSuite

	clients []string
}

// Make some clients in the index.
func (self *TestSuite) populatedClients() {
	search.ResetLRU()
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

func (self *TestSuite) TestMRU() {
	var err error

	// Make some clients
	self.populatedClients()

	full_walk := func() {
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
	}

	// Walk once
	full_walk()

	stats := search.LRUStats()
	assert.Equal(self.T(), int64(0), stats.Hits)

	// Walk again
	full_walk()

	new_stats := search.LRUStats()
	// No new looksups - everything should come from the cache.
	assert.Equal(self.T(), stats.Misses, new_stats.Misses)

	// All old misses come from cache this time.
	assert.Equal(self.T(), stats.Misses, new_stats.Hits)

	// Label two clients.
	err = search.SetIndex(self.ConfigObj, "C.452", "label:foobar")
	assert.NoError(self.T(), err)

	err = search.SetIndex(self.ConfigObj, "C.123", "label:foobar")
	assert.NoError(self.T(), err)

	labels := []string{}
	for hit := range search.SearchIndexWithPrefix(
		context.Background(), self.ConfigObj,
		"label:foobar", search.OPTION_ENTITY) {
		if hit != nil {
			client_id := hit.Entity
			labels = append(labels, client_id)
		}
	}

	// We find the clients by label (Results are sorted)
	assert.Equal(self.T(), []string{"C.123", "C.452"}, labels)

	// Searching for the new label has increased cache misses.
	stats = search.LRUStats()
	assert.True(self.T(), stats.Misses > new_stats.Misses)
}

func (self *TestSuite) TestMRUTimeExpiry() {
	// Make some clients
	self.populatedClients()

	full_walk := func() {
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
	}

	search.SetLRUClock(utils.MockClock{MockNow: time.Unix(100, 0)})
	full_walk()

	initial_miss_count := getLRUMissRate(self.T())

	// Now advance the clock 10 seconds
	search.SetLRUClock(utils.MockClock{MockNow: time.Unix(110, 0)})
	full_walk()

	// No misses - everything comes from cache within 60 seconds
	assert.Equal(self.T(), initial_miss_count, getLRUMissRate(self.T()))

	// Advance the clock 1 minute
	search.SetLRUClock(utils.MockClock{MockNow: time.Unix(171, 0)})
	full_walk()

	new_miss_count := getLRUMissRate(self.T())
	assert.True(self.T(), new_miss_count > initial_miss_count*2/3)
}

func (self *TestSuite) TestEnumerateIndex() {
	// Make some clients
	self.populatedClients()
	// test_utils.GetMemoryDataStore(self.T(), self.ConfigObj).Debug()

	// Measure how many ListChildren() operations are performed
	initial_op_count := getIndexListings(self.T())

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

	current_op_count := getIndexListings(self.T())
	// These numbers depend on the partition size.
	assert.Equal(self.T(), uint64(490), current_op_count-initial_op_count)

	// Now test that early exit reduces the number of listing
	// operations.
	initial_op_count = current_op_count

	// Only read one client
	ctx, cancel := context.WithCancel(context.Background())
	for hit := range search.SearchIndexWithPrefix(
		ctx, self.ConfigObj, "", search.OPTION_ENTITY) {
		if hit != nil {
			client_id := hit.Entity
			assert.Equal(self.T(), "C.0030300030303000", client_id)
			cancel()
			break
		}
	}

	current_op_count = getIndexListings(self.T())
	assert.True(self.T(), 50 > current_op_count-initial_op_count)
}

func getIndexListings(t *testing.T) uint64 {
	gathering, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)

	for _, metric := range gathering {
		if *metric.Name == "datastore_latency" {
			for _, m := range metric.Metric {
				if len(m.Label) >= 2 &&
					*m.Label[0].Value == "list" &&
					*m.Label[1].Value == "Index" {
					return *m.Histogram.SampleCount
				}
			}
		}
	}
	return 0
}

func getLRUMissRate(t *testing.T) uint64 {
	gathering, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)

	for _, metric := range gathering {
		if *metric.Name == "search_index_lru_miss" {
			for _, m := range metric.Metric {
				return uint64(*m.Counter.Value)
			}
		}
	}
	return 0
}

func TestIndexing(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
