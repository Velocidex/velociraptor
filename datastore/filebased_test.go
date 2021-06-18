package datastore

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
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
			clock: utils.MockClock{MockNow: time.Unix(100, 0)},
		},
		config_obj: config_obj,
	}})
}

func benchmarkSearchClientCount(b *testing.B, count int, sort_direction SortingSense) {
	dir, _ := ioutil.TempDir("", "datastore_test")
	defer os.RemoveAll(dir) // clean up

	db := &FileBaseDataStore{
		clock: utils.RealClock{},
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

/*
goos: linux
goarch: amd64
pkg: www.velocidex.com/golang/velociraptor/datastore
BenchmarkSearchClient
Creating 1000 clients on /tmp/datastore_test496793479 (Sorting 0)
BenchmarkSearchClient/All
BenchmarkSearchClient/All-16                 225           6403265 ns/op
BenchmarkSearchClient/By_Lables
BenchmarkSearchClient/By_Lables-16           180           5744585 ns/op
BenchmarkSearchClient/By_All_Lables
BenchmarkSearchClient/By_All_Lables-16                51          22922778 ns/op
BenchmarkSearchClient/By_hostname
BenchmarkSearchClient/By_hostname-16               23781             52901 ns/op
BenchmarkSearchClient/By_client_id
BenchmarkSearchClient/By_client_id-16              21888             55661 ns/op
BenchmarkSearchClient/By_All_hostnames
BenchmarkSearchClient/By_All_hostnames-16             49          23497222 ns/op
Creating 10000 clients on /tmp/datastore_test241135930 (Sorting 0)
BenchmarkSearchClient/All#01
BenchmarkSearchClient/All#01-16                       19          60842851 ns/op
BenchmarkSearchClient/By_Lables#01
BenchmarkSearchClient/By_Lables#01-16                 20          57002530 ns/op
BenchmarkSearchClient/By_All_Lables#01
BenchmarkSearchClient/By_All_Lables#01-16              4         263212968 ns/op
BenchmarkSearchClient/By_hostname#01
BenchmarkSearchClient/By_hostname#01-16            20967             53810 ns/op
BenchmarkSearchClient/By_client_id#01
BenchmarkSearchClient/By_client_id#01-16           24430             47961 ns/op
BenchmarkSearchClient/By_All_hostnames#01
BenchmarkSearchClient/By_All_hostnames#01-16           5         292543554 ns/op
Creating 1000 clients on /tmp/datastore_test427608913 (Sorting 1)
BenchmarkSearchClient/All#02
BenchmarkSearchClient/All#02-16                      132           8818529 ns/op
BenchmarkSearchClient/By_Lables#02
BenchmarkSearchClient/By_Lables#02-16                100          10459059 ns/op
BenchmarkSearchClient/By_All_Lables#02
BenchmarkSearchClient/By_All_Lables#02-16             12         107499717 ns/op
BenchmarkSearchClient/By_hostname#02
BenchmarkSearchClient/By_hostname#02-16            19784             55603 ns/op
BenchmarkSearchClient/By_client_id#02
BenchmarkSearchClient/By_client_id#02-16           23761             50316 ns/op
BenchmarkSearchClient/By_All_hostnames#02
BenchmarkSearchClient/By_All_hostnames#02-16          13          97149201 ns/op
Creating 10000 clients on /tmp/datastore_test299055740 (Sorting 1)
BenchmarkSearchClient/All#03
BenchmarkSearchClient/All#03-16                       13          81121369 ns/op
BenchmarkSearchClient/By_Lables#03
BenchmarkSearchClient/By_Lables#03-16                 13          84094550 ns/op
BenchmarkSearchClient/By_All_Lables#03
BenchmarkSearchClient/By_All_Lables#03-16              1        1362633032 ns/op
BenchmarkSearchClient/By_All_Lables#03
BenchmarkSearchClient/By_All_Lables#03-16              1        1362633032 ns/op
BenchmarkSearchClient/By_hostname#03
BenchmarkSearchClient/By_hostname#03-16            20892             53779 ns/op
BenchmarkSearchClient/By_client_id#03
BenchmarkSearchClient/By_client_id#03-16           24312             49224 ns/op
BenchmarkSearchClient/By_All_hostnames#03
BenchmarkSearchClient/By_All_hostnames#03-16           2        1010917148 ns/op
PASS
*/

func BenchmarkSearchClient(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		benchmarkSearchClientCount(b, count, UNSORTED)
	}

	for _, count := range []int{1000, 10000} {
		benchmarkSearchClientCount(b, count, SORT_UP)
	}
}
