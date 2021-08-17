package server_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/server"
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
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU/query_1.json.index`
)

type FilestoreTestSuite struct {
	test_utils.TestSuite

	client_id, flow_id string
}

func (self *FilestoreTestSuite) TestEnumerateFlow() {
	config_obj := self.ConfigObj
	db, err := datastore.GetDB(config_obj)
	assert.NoError(self.T(), err)

	file_store_factory := file_store.GetFileStore(config_obj)

	for _, line := range strings.Split(sample_flow, "\n") {
		line = "/clients/C.123/" + line
		if strings.HasSuffix(line, ".db") {
			db.SetSubject(self.ConfigObj, paths.DSPathSpecFromClientPath(line),
				&empty.Empty{})
		} else {
			path_spec := paths.FSPathSpecFromClientPath(line)
			fd, err := file_store_factory.WriteFile(path_spec)
			assert.NoError(self.T(), err)
			fd.Write([]byte("X"))
			fd.Close()
		}
	}

	// Populate the client's space with some data.
	client_info := &actions_proto.ClientInfo{
		ClientId: self.client_id,
	}
	client_path_manager := paths.NewClientPathManager(self.client_id)
	flow_pm := client_path_manager.Flow(self.flow_id)

	db.SetSubject(self.ConfigObj,
		client_path_manager.Path(), client_info)

	flow_context := &flows_proto.ArtifactCollectorContext{
		SessionId: self.flow_id,
	}
	db.SetSubject(self.ConfigObj, flow_pm.Path(), flow_context)
	db.SetSubject(self.ConfigObj, flow_pm.Task(), client_info)

	// Write some filestore files
	fd, _ := file_store_factory.WriteFile(flow_pm.GetUploadsFile(
		"ntfs", `\\.\C:\Windows\notepad.exe`).Path())
	fd.Close()

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

	result := vtesting.RunPlugin(server.EnumerateFlowPlugin{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("flow_id", self.flow_id).
			Set("client_id", self.client_id)))

	goldie.Assert(self.T(), "TestEnumerateFlow", json.MustMarshalIndent(result))
}

func TestFilestorePlugin(t *testing.T) {
	suite.Run(t, &FilestoreTestSuite{
		client_id: "C.123",
		flow_id:   "F.1234",
	})
}
