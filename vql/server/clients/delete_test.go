package clients

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

var (
	sample_flow = `collections/F.1234/task.db
collections/F.1234/uploads/ntfs/%3A%3A.%3AC/Windows/notepad.exe
collections/F.1234/logs
collections/F.1234/logs.json.index
collections/F.1234/uploads.json
collections/F.1234/uploads.json.index
collections/F.1234/notebook/N.F.1234-C.123.json.db
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT16FBL4PM.json.db
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU.json.db
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU/query_1.json
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU/query_1.json.index
monitoring_logs/Generic.Client.Stats/2021-08-14.json
monitoring_logs/Generic.Client.Stats/2021-08-14.json.tidx
labels.json.db
vfs/file.db
vfs/file/C%3A.db
vfs/file/C%3A/Users.db
vfs_files/file/C%3A/Users/mike/test/1.txt.db
artifacts/Generic.Client.Info/F.C49TC44OSO62E/Users.json
artifacts/Generic.Client.Info/F.C49TC44OSO62E/Users.json.index
artifacts/Generic.Client.Info/F.C49TC44OSO62E/BasicInformation.json
artifacts/Generic.Client.Info/F.C49TC44OSO62E/BasicInformation.json.index
tasks/task1123.db
ping.db`
)

type DeleteTestSuite struct {
	test_utils.TestSuite
	client_id string
}

func (self *DeleteTestSuite) TestDeleteClient() {
	ConfigObj := self.ConfigObj
	db, err := datastore.GetDB(ConfigObj)
	assert.NoError(self.T(), err)

	file_store_factory := file_store.GetFileStore(ConfigObj)

	for _, line := range strings.Split(sample_flow, "\n") {
		line = "/clients/C.123/" + line
		if strings.HasSuffix(line, ".db") {
			db.SetSubject(self.ConfigObj, paths.DSPathSpecFromClientPath(line),
				&empty.Empty{})
		} else {
			path_spec := paths.FSPathSpecFromClientPath(line)
			fd, err := file_store_factory.WriteFile(path_spec)
			assert.NoError(self.T(), err)
			fd.Close()
		}
	}

	manager, _ := services.GetRepositoryManager()
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger: logging.NewPlainLogger(self.ConfigObj,
			&logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	result := vtesting.RunPlugin(DeleteClientPlugin{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("really_do_it", true).
			Set("client_id", self.client_id)))

	test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
	json.Dump(result)
}

func TestDeletePlugin(t *testing.T) {
	suite.Run(t, &DeleteTestSuite{
		client_id: "C.123",
	})
}
