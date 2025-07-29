package datastore_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type RemoteTestSuite struct {
	test_utils.TestSuite
}

func (self *RemoteTestSuite) SetupTest() {
	var err error
	os.Setenv("VELOCIRAPTOR_LITERAL_CONFIG", test_utils.SERVER_CONFIG)
	self.ConfigObj, err = new(config.Loader).
		WithEnvLiteralLoader(constants.VELOCIRAPTOR_LITERAL_CONFIG).
		WithRequiredFrontend().
		WithVerbose(true).LoadAndValidate()
	require.NoError(self.T(), err)

	dir, err := tempfile.TempDir("file_store_test")
	assert.NoError(self.T(), err)

	self.ConfigObj.Datastore.Implementation = "FileBaseDataStore"
	self.ConfigObj.Datastore.FilestoreDirectory = dir
	self.ConfigObj.Datastore.Location = dir

	free_port, err := vtesting.GetFreePort()
	assert.NoError(self.T(), err)

	fmt.Printf("API port will be %v\n", free_port)

	self.ConfigObj.API = &config_proto.APIConfig{
		BindPort:    uint32(free_port),
		BindAddress: "127.0.0.1",
		BindScheme:  "tcp",
	}

	self.TestSuite.SetupTest()

	// Reset the api clients
	grpc_client.Factory = &grpc_client.DummyGRPCAPIClient{}
}

func (self *RemoteTestSuite) startAPIServer() {
	builder, err := api.NewServerBuilder(self.Ctx, self.ConfigObj, self.Sm.Wg)
	assert.NoError(self.T(), err)

	err = builder.WithAPIServer(self.Ctx, self.Sm.Wg)
	assert.NoError(self.T(), err)
}

func (self *RemoteTestSuite) TearDownTest() {
	os.RemoveAll(self.ConfigObj.Datastore.FilestoreDirectory)
}

func (self *RemoteTestSuite) TestRemoteDataStore() {
	datastore.RPC_BACKOFF = 0.5
	self.startAPIServer()

	db := datastore.NewRemoteDataStore(self.Ctx)

	client_info := &api_proto.ApiClient{ClientId: "C.1234"}
	client_path_manager := paths.NewClientPathManager(client_info.ClientId)

	err := db.GetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
	assert.Error(self.T(), err)

	err = db.SetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
	assert.NoError(self.T(), err)

	err = db.GetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
	assert.NoError(self.T(), err)
}

// Tgest retry when connecting to
func (self *RemoteTestSuite) TestRemoteDataStoreMissing() {
	if testing.Short() {
		self.T().Skip("skipping test in short mode.")
	}

	datastore.RPC_BACKOFF = 0
	datastore.RPC_RETRY = 2
	logging.ClearMemoryLogs()

	db := datastore.NewRemoteDataStore(self.Ctx)

	client_info := &api_proto.ApiClient{ClientId: "C.1234"}
	client_path_manager := paths.NewClientPathManager(client_info.ClientId)

	err := db.GetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
	assert.Error(self.T(), err)

	err = db.SetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
	assert.Error(self.T(), err)

	err = db.GetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
	assert.Error(self.T(), err)

	matches := []string{}
	for _, line := range logging.GetMemoryLogs() {
		if strings.Contains(line, "code = Unavailable desc = connection error") {
			matches = append(matches, line)
		}
	}

	// We had at least 5 retries to the various calls
	assert.True(self.T(), len(matches) > 5)
}

func TestRemoteTestSuite(t *testing.T) {
	suite.Run(t, &RemoteTestSuite{})
}
