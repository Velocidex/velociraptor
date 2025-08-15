package flows_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/emptypb"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/server/flows"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

var (
	sample_flow = `collections/F.1234/task.db
collections/F.1234/logs.json
collections/F.1234/logs.json.index
collections/F.1234/logs.chunk
collections/F.1234/stats.json.db
collections/F.1234/task.db
collections/F.1234/upload_transactions.json
collections/F.1234/upload_transactions.json.index
collections/F.1234/uploads.json
collections/F.1234/uploads.json.index
collections/F.1234/uploads.chunk
collections/F.1234/uploads/ntfs/\\.\C:/Windows/notepad.exe
collections/F.1234/uploads/ntfs/\\.\C:/Windows/notepad.exe.idx
collections/F.1234/uploads/ntfs/\\.\C:/Windows/notepad.exe.chunk
collections/F.1234/notebook/N.F.1234-C.123.json.db
collections/F.1234/notebook/N.F.1234-C.123/artifact.json.db
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT16FBL4PM.json.db
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU.json.db
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU/query_1.json
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU/query_1.json.index
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU/logs.json
collections/F.1234/notebook/N.F.1234-C.123/NC.C4BKT1195IMMU/logs.json.index
`
)

type FilestoreTestSuite struct {
	test_utils.TestSuite

	client_id, flow_id string
}

func (self *FilestoreTestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.NotebookService = true
	self.ConfigObj.Services.SchedulerService = true

	self.TestSuite.SetupTest()
}

func (self *FilestoreTestSuite) initFlowData() {
	config_obj := self.ConfigObj
	db, err := datastore.GetDB(config_obj)
	assert.NoError(self.T(), err)

	file_store_factory := file_store.GetFileStore(config_obj)

	for _, line := range strings.Split(sample_flow, "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		line = "clients/C.123/" + line

		// Data store files.
		if strings.HasSuffix(line, ".db") ||
			strings.HasSuffix(line, ".json.db") {
			db.SetSubject(self.ConfigObj,
				paths.DSPathSpecFromClientPath(line), &emptypb.Empty{})

		} else {
			path_spec := path_specs.NewUnsafeFilestorePath(
				strings.Split(line, "/")...).
				SetType(api.PATH_TYPE_FILESTORE_ANY)

			fd, err := file_store_factory.WriteFile(path_spec)
			assert.NoError(self.T(), err)

			fd.Write([]byte("X"))
			fd.Close()
		}
	}

	flow_pm := paths.NewFlowPathManager(self.client_id, self.flow_id)
	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		flow_pm.UploadMetadata(),
		nil, utils.SyncCompleter, result_sets.TruncateMode)
	assert.NoError(self.T(), err)

	rs_writer.Write(ordereddict.NewDict().
		Set("_Components", []string{
			"clients", "C.123", "collections", "F.1234",
			"uploads", "ntfs", `\\.\C:`,
			"Windows", "notepad.exe"}))
	rs_writer.Close()

	// Populate the client's space with some data.
	client_info := &actions_proto.ClientInfo{
		ClientId: self.client_id,
	}

	client_path_manager := paths.NewClientPathManager(self.client_id)
	db.SetSubject(self.ConfigObj,
		client_path_manager.Path(), client_info)

	flow_context := &flows_proto.ArtifactCollectorContext{
		SessionId: self.flow_id,
	}
	db.SetSubject(self.ConfigObj, flow_pm.Path(), flow_context)
	db.SetSubject(self.ConfigObj, flow_pm.Task(), client_info)
}

func (self *FilestoreTestSuite) TestEnumerateFlow() {
	self.initFlowData()

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(self.ConfigObj,
			&logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	result := vtesting.RunPlugin(flows.EnumerateFlowPlugin{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("flow_id", self.flow_id).
			Set("client_id", self.client_id)))
	goldie.AssertJson(self.T(), "TestEnumerateFlow", result)
}

func (self *FilestoreTestSuite) TestDeleteFlow() {
	self.initFlowData()

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(self.ConfigObj,
			&logging.FrontendComponent),
		Env: ordereddict.NewDict(),
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	result := vtesting.RunPlugin(flows.DeleteFlowPlugin{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("flow_id", self.flow_id).
			Set("client_id", self.client_id).
			Set("really_do_it", true).
			Set("sync", true)))

	var resident_paths []string
	for _, line := range test_utils.GetMemoryFileStore(
		self.T(), self.ConfigObj).Paths.Keys() {
		if strings.HasPrefix(line, "/clients/C.123/") {
			resident_paths = append(resident_paths, line)
		}
	}

	// Only the flow index should remain - all other files should be removed.
	result = append(result, ordereddict.NewDict().
		Set("resident_paths", resident_paths))

	goldie.AssertJson(self.T(), "TestDeleteFlow", result)

}

func TestFilestorePlugin(t *testing.T) {
	suite.Run(t, &FilestoreTestSuite{
		client_id: "C.123",
		flow_id:   "F.1234",
	})
}
