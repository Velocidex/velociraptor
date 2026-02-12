package api_test

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"github.com/stretchr/testify/suite"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/file_store"
	file_store_api "www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
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

	FALSE = false
)

// Tests the public API endpoints
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

	// Reset the connection pool for each test.
	grpc_client.Factory = &grpc_client.DummyGRPCAPIClient{}

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
			self.Sm.Ctx, grpc_client.API_User, self.ConfigObj)
		assert.NoError(self.T(), err)
		defer closer()

		res, err := conn.Check(self.Sm.Ctx, &api_proto.HealthCheckRequest{})
		return err == nil && res.Status == api_proto.HealthCheckResponse_SERVING
	})
}

func (self *GeneralAPITest) TestQuery() {
	// Create the user
	user_manager := services.GetUserManager()
	err := user_manager.SetUser(self.Ctx, &api_proto.VelociraptorUser{
		Name: self.username,
	})

	// Make the user a reader on the root org.
	err = services.GrantUserToOrg(self.Ctx,
		utils.GetSuperuserName(self.ConfigObj),
		self.username,
		[]string{"root"}, &acl_proto.ApiClientACL{
			Roles: []string{"reader"},
			// User is not permitted to access over the API.
		})
	assert.NoError(self.T(), err)

	message := &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			{VQL: "SELECT * FROM info()"},
		},
	}

	resp, err := self.getQueryResults(message)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(),
		"Permission denied: User TestApiUser requires permission ANY_QUERY")
	assert.Equal(self.T(), len(resp), 0)

	// Now give the user the any query permission.
	err = services.GrantUserToOrg(self.Ctx,
		utils.GetSuperuserName(self.ConfigObj),
		self.username,
		[]string{"root"}, &acl_proto.ApiClientACL{
			Roles:    []string{"reader"},
			AnyQuery: true,
		})
	assert.NoError(self.T(), err)

	// Try again: Query is allowed to run now but it is running with
	// reduced permissions. The VQL engine itself will enforce
	// permissions.
	resp, err = self.getQueryResults(message)
	assert.NoError(self.T(), err)
	assert.True(self.T(), self.containsLog(
		resp, "PermissionDenied: Permission denied: [MACHINE_STATE]"),
		json.MustMarshalString(resp))

	// Now give the user also the MACHINE_STATE permission.
	err = services.GrantUserToOrg(self.Ctx,
		utils.GetSuperuserName(self.ConfigObj),
		self.username,
		[]string{"root"}, &acl_proto.ApiClientACL{
			Roles:        []string{"reader"},
			AnyQuery:     true,
			MachineState: true,
		})
	assert.NoError(self.T(), err)

	// It works fine now.
	resp, err = self.getQueryResults(message)
	assert.NoError(self.T(), err)
	assert.True(self.T(), !self.containsLog(resp, "PermissionDenied"),
		json.MustMarshalString(resp))
}

func (self *GeneralAPITest) containsLog(resp []*actions_proto.VQLResponse, log string) bool {
	for _, r := range resp {
		if strings.Contains(r.Log, log) {
			return true
		}
	}
	return false
}

func (self *GeneralAPITest) getQueryResults(
	message *actions_proto.VQLCollectorArgs) (
	res []*actions_proto.VQLResponse, err error) {

	client, closer, err := grpc_client.Factory.GetAPIClient(
		self.Ctx, grpc_client.API_User, self.ConfigObj)
	assert.NoError(self.T(), err)

	defer closer()

	receiver, err := client.Query(self.Ctx, message)
	if err != nil {
		return nil, err
	}

	for {
		response, err := receiver.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return res, err
		}
		res = append(res, response)
	}

	return res, nil
}

func (self *GeneralAPITest) TestVFSGetBuffer() {
	filename := path_specs.NewSafeFilestorePath("A", "B", "C").
		SetType(file_store_api.PATH_TYPE_FILESTORE_ANY)

	// Write some data to the filestore
	file_store_factory := file_store.GetFileStore(self.ConfigObj)
	writer, err := file_store_factory.WriteFile(filename)
	assert.NoError(self.T(), err)

	_, err = writer.Write([]byte("Hello"))
	assert.NoError(self.T(), err)

	writer.Close()

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
			// No permissions
			Roles: []string{},
		})
	assert.NoError(self.T(), err)

	client, closer, err := grpc_client.Factory.GetAPIClient(
		self.Ctx, grpc_client.API_User, self.ConfigObj)
	assert.NoError(self.T(), err)

	defer closer()

	message := &api_proto.VFSFileBuffer{
		Components: filename.Components(),
		Length:     100,
	}

	buf, err := client.VFSGetBuffer(self.Ctx, message)
	assert.Error(self.T(), err)
	assert.Contains(self.T(), err.Error(),
		"PermissionDenied desc = User is not allowed to view the VFS")

	// Give permission and try again.
	err = services.GrantUserToOrg(self.Ctx,
		utils.GetSuperuserName(self.ConfigObj),
		self.username,
		[]string{"root"}, &acl_proto.ApiClientACL{
			Roles: []string{"reader"},
		})
	assert.NoError(self.T(), err)

	buf, err = client.VFSGetBuffer(self.Ctx, message)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), string(buf.Data), "Hello")
}

func (self *GeneralAPITest) TestVFSGetBufferSparse() {
	// Create a sparse file that contains "HelloWorld"
	filename := path_specs.FromGenericComponentList([]string{"sparse_upload.txt"}).
		SetType(file_store_api.PATH_TYPE_FILESTORE_ANY)
	filename_idx := filename.SetType(file_store_api.PATH_TYPE_FILESTORE_SPARSE_IDX)

	file_store_factory := file_store.GetFileStore(self.ConfigObj)

	w, err := file_store_factory.WriteFile(filename)
	assert.NoError(self.T(), err)
	w.Write([]byte("HelloWorld"))
	w.Close()

	// Only 10 bytes are written to the filestore.
	stat_file, err := file_store_factory.StatFile(filename)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), stat_file.Size(), int64(10))

	w, err = file_store_factory.WriteFile(filename_idx)
	assert.NoError(self.T(), err)

	// Original offset refers to the offset in the remote sparse file.
	// file offset refers to the offset within the filestore file
	w.Write([]byte(`
{
 "ranges": [
  {
   "file_offset": 0,
   "original_offset": 0,
   "file_length": 5,
   "length": 5
  },
  {
   "file_offset": 5,
   "original_offset": 5,
   "length": 5,
   "file_length": 0
  },
  {
   "file_offset": 5,
   "original_offset": 10,
   "file_length": 5,
   "length": 5
  }
 ]
}`)) // This represents: Hello<.....>World with the gap being sparse.
	w.Close()

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

	client, closer, err := grpc_client.Factory.GetAPIClient(
		self.Ctx, grpc_client.API_User, self.ConfigObj)
	assert.NoError(self.T(), err)

	defer closer()

	// Read padded buffer this should default to padding = true
	buf, err := client.VFSGetBuffer(self.Ctx, &api_proto.VFSFileBuffer{
		Components: filename.Components(),
		Length:     100,
	})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), string(buf.Data), "Hello\x00\x00\x00\x00\x00World")

	// Read unpadded buffer
	buf, err = client.VFSGetBuffer(self.Ctx, &api_proto.VFSFileBuffer{
		Components: filename.Components(),
		Length:     100,
		Padding:    &FALSE,
	})
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), string(buf.Data), "HelloWorld")
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
			// User has no permissions at all!
			Roles: []string{},
		})
	assert.NoError(self.T(), err)

	message := &api_proto.PushEventRequest{
		Artifact: "Server.Audit.Logs",
		Jsonl:    append([]byte(`{"foo": "bar"}`), '\n'),
		Rows:     1,
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
			// User has no roles at all! but should still be able to
			// push to the audit log.
			Roles:         []string{},
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
