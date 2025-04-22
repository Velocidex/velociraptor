package api_test

import (
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var (
	definitions = []string{`
name: Server.Audit.Logs
type: SERVER_EVENT
`}
)

type GeneralAPITest struct {
	test_utils.TestSuite

	client_config *config_proto.Config
	username      string
}

func (self *GeneralAPITest) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.API.BindPort = 8787

	self.LoadArtifactsIntoConfig(definitions)

	self.TestSuite.SetupTest()

	// Generate an API client.
	self.username = "TestApiUser"
	bundle, err := crypto.GenerateServerCert(
		self.ConfigObj, self.username)
	assert.NoError(self.T(), err)

	self.ConfigObj.ApiConfig = &config_proto.ApiClientConfig{
		CaCertificate:    self.ConfigObj.Client.CaCertificate,
		ClientCert:       bundle.Cert,
		ClientPrivateKey: string(bundle.PrivateKey),
		Name:             self.username,
	}

	server_builder, err := api.NewServerBuilder(
		self.Sm.Ctx, self.ConfigObj, self.Sm.Wg)
	assert.NoError(self.T(), err)

	err = server_builder.WithAPIServer(self.Sm.Ctx, self.Sm.Wg)
	assert.NoError(self.T(), err)

	// Now bring up an API server.
	self.ConfigObj.Services = &config_proto.ServerServicesConfig{}

	// Wait for the server to come up.
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		conn, closer, err := grpc_client.Factory.GetAPIClient(
			self.Sm.Ctx, grpc_client.SuperUser, self.ConfigObj)
		assert.NoError(self.T(), err)
		defer closer()

		res, err := conn.Check(self.Sm.Ctx, &api_proto.HealthCheckRequest{})
		return err == nil && res.Status == api_proto.HealthCheckResponse_SERVING
	})
}

func (self *GeneralAPITest) TestPushEvents() {
	client, closer, err := grpc_client.Factory.GetAPIClient(
		self.Ctx, grpc_client.API_User, self.ConfigObj)
	assert.NoError(self.T(), err)

	defer closer()

	// Create the user
	user_manager := services.GetUserManager()
	err = user_manager.SetUser(self.Ctx, &api_proto.VelociraptorUser{
		Name: self.username,
	})

	// Make the user a reader on the root org.
	err = services.GrantUserToOrg(self.Ctx,
		utils.GetSuperuserName(self.ConfigObj),
		self.username,
		[]string{"root"}, &acl_proto.ApiClientACL{
			Roles: []string{"reader"},
		})
	assert.NoError(self.T(), err)

	message := &api_proto.PushEventRequest{
		Artifact: "Server.Audit.Logs",
		Jsonl:    append([]byte(`{"foo": "bar"}`), '\n'),
		Rows:     1,
		Write:    true,
	}

	// Try to push the event - should not work because user has no
	// publish access.
	_, err = client.PushEvents(self.Ctx, message)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(),
		"Permission denied: PUBLISH TestApiUser to Server.Audit.Logs")

	// Give the user publish access to this queue.
	err = services.GrantUserToOrg(self.Ctx,
		utils.GetSuperuserName(self.ConfigObj),
		self.username,
		[]string{"root"}, &acl_proto.ApiClientACL{
			Roles:         []string{"reader"},
			PublishQueues: []string{"Server.Audit.Logs"},
		})
	assert.NoError(self.T(), err)

	// Try again - should work this time!
	_, err = client.PushEvents(self.Ctx, message)
	assert.NoError(self.T(), err)

	// Lets check if it is there.

	path_manager := artifacts.NewArtifactPathManagerWithMode(
		self.ConfigObj, "server", "", "Server.Audit.Logs",
		paths.MODE_SERVER_EVENT)

	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	rs_reader, err := result_sets.NewResultSetReaderWithOptions(
		self.Ctx, self.ConfigObj, file_store_factory,
		path_manager.Path(), result_sets.ResultSetOptions{})
	assert.NoError(self.T(), err)
	defer rs_reader.Close()

	var rows []*ordereddict.Dict
	for row := range rs_reader.Rows(self.Ctx) {
		rows = append(rows, row)
	}

	assert.Equal(self.T(), len(rows), 1)

	// Make sure the user that sent the event is marked in the event.
	sender, _ := rows[0].GetString("_Sender")
	assert.Equal(self.T(), sender, self.username)

	// The actual data is stored.
	bar, _ := rows[0].GetString("foo")
	assert.Equal(self.T(), bar, "bar")
}

func TestAPI(t *testing.T) {
	suite.Run(t, &GeneralAPITest{})
}
