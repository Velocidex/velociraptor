package api

import (
	"testing"

	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type DatastoreAPITest struct {
	test_utils.TestSuite

	client_config *config_proto.Config
}

func (self *DatastoreAPITest) SetupTest() {
	self.TestSuite.SetupTest()

	// Now bring up an API server.
	self.ConfigObj.Frontend.ServerServices = &config_proto.ServerServicesConfig{}
	self.ConfigObj.API.BindPort = 8101

	server_builder, err := NewServerBuilder(
		self.Sm.Ctx, self.ConfigObj, self.Sm.Wg)
	assert.NoError(self.T(), err)

	err = server_builder.WithAPIServer(self.Sm.Ctx, self.Sm.Wg)
	assert.NoError(self.T(), err)

}

func (self *DatastoreAPITest) TestDatastore() {
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	path_spec := path_specs.NewUnsafeDatastorePath("A", "B", "C")
	sample := &api_proto.AgentInformation{Name: "Velociraptor"}
	assert.NoError(self.T(),
		db.SetSubject(self.ConfigObj, path_spec, sample))

	// Make some RPC calls
	conn, closer, err := grpc_client.Factory.GetAPIClient(
		self.Sm.Ctx, self.ConfigObj)
	assert.NoError(self.T(), err)
	defer closer()

	res, err := conn.GetSubject(self.Sm.Ctx, &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: path_spec.Components(),
		}})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), res.Data, []byte("{\"name\":\"Velociraptor\"}"))

	// Now set data through gRPC and read it using the standard
	// datastore.
	path_spec2 := path_specs.NewUnsafeDatastorePath("A", "B", "D")
	_, err = conn.SetSubject(self.Sm.Ctx, &api_proto.DataRequest{
		Data: []byte("{\"name\":\"Another Name\"}"),
		Pathspec: &api_proto.DSPathSpec{
			Components: path_spec2.Components(),
		}})
	assert.NoError(self.T(), err)

	assert.NoError(self.T(),
		db.GetSubject(self.ConfigObj, path_spec2, sample))
	assert.Equal(self.T(), sample.Name, "Another Name")

	// Now list the children
	res2, err := conn.ListChildren(self.Sm.Ctx, &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: path_spec.Dir().Components(),
		}})
	assert.NoError(self.T(), err)
	goldie.Assert(self.T(), "TestDatastore", json.MustMarshalIndent(res2))
}

func (self *DatastoreAPITest) TestRemoteDatastore() {
	config_obj := proto.Clone(self.ConfigObj).(*config_proto.Config)
	config_obj.Datastore.Implementation = "RemoteFileDataStore"

	db, err := datastore.GetDB(config_obj)
	assert.NoError(self.T(), err)

	path_spec := path_specs.NewUnsafeDatastorePath("A", "B", "C")
	sample := &api_proto.AgentInformation{Name: "Velociraptor"}
	assert.NoError(self.T(),
		db.SetSubject(config_obj, path_spec, sample))

	sample2 := &api_proto.AgentInformation{}
	assert.NoError(self.T(),
		db.GetSubject(config_obj, path_spec, sample2))

	assert.Equal(self.T(), sample, sample2)

	// Test ListDirectory
	path_spec2 := path_specs.NewUnsafeDatastorePath("A", "B", "D")
	assert.NoError(self.T(),
		db.SetSubject(config_obj, path_spec2, sample))

	children, err := db.ListChildren(config_obj, path_spec.Dir())
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 2, len(children))
	assert.Equal(self.T(), path_spec, children[0])
	assert.Equal(self.T(), path_spec2, children[1])

	// Now delete one
	assert.NoError(self.T(),
		db.DeleteSubject(config_obj, path_spec))

	children, err = db.ListChildren(config_obj, path_spec.Dir())
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(children))
	assert.Equal(self.T(), path_spec2, children[0])

	children, err = db.ListChildren(config_obj, path_spec.Dir().Dir())
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(children))
	assert.True(self.T(), children[0].IsDir())
}

func TestAPIDatastore(t *testing.T) {
	suite.Run(t, &DatastoreAPITest{})
}
