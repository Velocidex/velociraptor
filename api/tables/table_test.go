package tables_test

import (
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/api/tables"
	"www.velocidex.com/golang/velociraptor/constants"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type TestSuite struct {
	test_utils.TestSuite
}

func (self *TestSuite) SetupTest() {
	// Start the sanity checker to install filestore denied prefixes.
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.SanityChecker = true

	self.TestSuite.SetupTest()
}

func (self *TestSuite) TestFlowLogsTable() {
	client_id := "C.123"
	flow_id := "F.1234"

	path_manager := paths.NewFlowPathManager(client_id, flow_id)
	self.WriteResultSet(path_manager.Log())

	res, err := tables.GetTable(self.Ctx, self.ConfigObj,
		&api_proto.GetTableRequest{
			ClientId: client_id,
			FlowId:   flow_id,
			Type:     "log",
		},
		constants.VELOCIRAPTOR_SERVER_CLIENT_ID)
	assert.NoError(self.T(), err)

	goldie.Assert(self.T(), "TestFlowLogsTable",
		json.MustMarshalIndent(res))
}

func (self *TestSuite) TestStackTable() {
	path := path_specs.NewUnsafeFilestorePath(
		"config", "Bar", "Baz", "stack")
	self.WriteResultSet(path)

	_, err := tables.GetTable(self.Ctx, self.ConfigObj,
		&api_proto.GetTableRequest{
			Type: "STACK",
			StackPath: []string{
				"config", "Bar", "Baz", "stack",
			},
		},
		constants.VELOCIRAPTOR_SERVER_CLIENT_ID)
	assert.Error(self.T(), err)
	assert.ErrorContains(self.T(), err, "No access to filesystem path")
}

func (self *TestSuite) WriteResultSet(path api.FSPathSpec) {
	file_store_factory := file_store.GetFileStore(self.ConfigObj)

	rs, err := result_sets.NewResultSetWriter(
		file_store_factory, path, nil, utils.SyncCompleter, true)
	assert.NoError(self.T(), err)
	rs.Write(ordereddict.NewDict().Set("Foo", 1))
	rs.Write(ordereddict.NewDict().Set("Foo", 2))
	rs.Write(ordereddict.NewDict().Set("Foo", 3))

	// Writes may not occur until the Close()
	rs.Close()
}

func TestTableAPI(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
