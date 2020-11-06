package datastore

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type FilebasedTestSuite struct {
	BaseTestSuite
}

func TestFilebasedDatabase(t *testing.T) {
	dir, err := ioutil.TempDir("", "datastore_test")
	assert.NoError(t, err)

	defer os.RemoveAll(dir) // clean up

	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.FilestoreDirectory = dir
	config_obj.Datastore.Location = dir

	suite.Run(t, &FilebasedTestSuite{BaseTestSuite{
		datastore: &FileBaseDataStore{
			clock: vtesting.RealClock{},
		},
		config_obj: config_obj,
	}})
}

func benchmarkSearchClientCount(b *testing.B, count int, sort_direction SortingSense) {
	dir, _ := ioutil.TempDir("", "datastore_test")
	defer os.RemoveAll(dir) // clean up

	db := &FileBaseDataStore{
		clock: vtesting.RealClock{},
	}

	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.FilestoreDirectory = dir
	config_obj.Datastore.Location = dir

	fmt.Printf("Creating %v clients on %v (Sorting %v)\n", count, dir, sort_direction)
	for i := 0; i < count; i++ {
		client_id := fmt.Sprintf("C.%08x", i)
		client_info := &actions_proto.ClientInfo{
			ClientId: client_id,
			Hostname: fmt.Sprintf("Host %08x", i),
			Fqdn:     fmt.Sprintf("Host %08x", i),
		}
		client_path_manager := paths.NewClientPathManager(client_id)
		err := db.SetSubject(config_obj, client_path_manager.Path(), client_info)
		if err != nil {
			fmt.Printf("Failed %v\n", err)
			b.FailNow()
		}

		// Update the client indexes for the GUI. Add any keywords we
		// wish to be searchable in the UI here.
		keywords := []string{
			"all", // This is used for "." search
			client_id,
			client_info.Hostname,
			client_info.Fqdn,
			"host:" + client_info.Hostname,
			"label:foo",
			"label:Host" + client_id,
		}

		err = db.SetIndex(config_obj, constants.CLIENT_INDEX_URN,
			client_id, keywords)
		if err != nil {
			fmt.Printf("Failed %v\n", err)
			b.FailNow()
		}
	}

	benchmarks := []struct {
		name     string
		query    string
		expected int
	}{
		{"All", "all", 10},
		{"By Lables", "label:foo", 10},
		{"By All Lables", "label:*", 10},
		{"By hostname", "host:Host 00000004", 1},
		{"By client id", "C.00000004", 1},
		{"By All hostnames", "host:*", 10},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				// Only retrieve the first 10 clients for the first page.
				hits := db.SearchClients(
					config_obj, constants.CLIENT_INDEX_URN,
					bm.query, "", 0, 10, sort_direction)
				if len(hits) != bm.expected {
					fmt.Printf("Got %v hits (%v) expected %v\n",
						len(hits), bm.query, bm.expected)
				}
			}
		})
	}
}

func BenchmarkSearchClient(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		benchmarkSearchClientCount(b, count, UNSORTED)
	}

	for _, count := range []int{1000, 10000} {
		benchmarkSearchClientCount(b, count, SORT_UP)
	}
}
