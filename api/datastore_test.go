package api_test

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/acls"
	acls_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/datastore"
	file_store_api "www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type DatastoreAPITest struct {
	test_utils.TestSuite
}

func (self *DatastoreAPITest) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.API.BindPort = 8787
	// Sanity checker sets up filestore allowed/denied sets
	self.ConfigObj.Services.SanityChecker = true

	self.TestSuite.SetupTest()

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

func (self *DatastoreAPITest) TestDatastore() {
	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	path_spec := path_specs.NewUnsafeDatastorePath("A", "B", "C")
	sample := &api_proto.AgentInformation{Name: "Velociraptor"}
	assert.NoError(self.T(),
		db.SetSubject(self.ConfigObj, path_spec, sample))

	// Make some RPC calls
	conn, closer, err := grpc_client.Factory.GetAPIClient(
		self.Sm.Ctx, grpc_client.SuperUser, self.ConfigObj)
	assert.NoError(self.T(), err)
	defer closer()

	test_utils.GetMemoryDataStore(self.T(), self.ConfigObj).Debug(self.ConfigObj)

	res, err := conn.GetSubject(self.Sm.Ctx, &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: path_spec.Components(),
			PathType:   int64(path_spec.Type()),
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

func (self *DatastoreAPITest) TestACLS() {
	username := "TestUser"

	bundle, err := crypto.GenerateServerCert(self.ConfigObj, username)
	assert.NoError(self.T(), err)

	// The peer is taken from the TLS connection - the peer
	// certificate is the username that we trust.
	peer_info := &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{
					bundle.Certificate,
				},
			},
		},
	}

	// Build a mocked context to emulate the one the grpc server builds.
	ctx := peer.NewContext(self.Ctx, peer_info)

	user_manager := services.GetUserManager()
	err = user_manager.SetUser(ctx, &api_proto.VelociraptorUser{
		Name: username,
	})
	assert.NoError(self.T(), err)

	// Check if the user can read the datastore
	path_spec := path_specs.NewUnsafeFilestorePath("clients", "C.1234", "foobar")
	_, principal, err := api.CheckDataStorePermissions(
		ctx, "", path_spec, acls.READ_RESULTS)
	assert.Error(self.T(), err)
	assert.ErrorContains(self.T(), err, "ACLs for user TestUser not found")

	// Grant the user read_access
	err = user_manager.AddUserToOrg(ctx, services.AddNewUser,
		utils.GetSuperuserName(self.ConfigObj),
		username, []string{"root"}, &acls_proto.ApiClientACL{
			ReadResults: true,
		})
	assert.NoError(self.T(), err)

	// User is allowed to read from the datastore
	_, principal, err = api.CheckDataStorePermissions(
		ctx, "root", path_spec, acls.READ_RESULTS)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), username, principal)

	// But they are not allowed to read private areas
	path_spec_private := path_specs.NewUnsafeFilestorePath("config", "foobar")
	_, principal, err = api.CheckDataStorePermissions(
		ctx, "root", path_spec_private, acls.READ_RESULTS)
	assert.Error(self.T(), err)
	assert.ErrorContains(self.T(), err, "No access to filesystem path")

	// Now give the user DATASTORE_ACCESS which gives them unfettered
	// access to all paths in the datastore.
	err = user_manager.AddUserToOrg(ctx, services.AddNewUser,
		utils.GetSuperuserName(self.ConfigObj),
		username, []string{"root"}, &acls_proto.ApiClientACL{
			ReadResults:     true,
			DatastoreAccess: true,
		})
	assert.NoError(self.T(), err)

	_, principal, err = api.CheckDataStorePermissions(
		ctx, "root", path_spec_private, acls.READ_RESULTS)
	assert.NoError(self.T(), err)
}

func (self *DatastoreAPITest) TestDatastoreAPI() {
	user_manager := services.GetUserManager()
	reader_username := "ReaderUser"

	bundle, err := crypto.GenerateServerCert(self.ConfigObj, reader_username)
	assert.NoError(self.T(), err)

	// Build a mocked context to emulate the one the grpc server builds.
	reader_ctx := peer.NewContext(self.Ctx, &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{
					bundle.Certificate,
				},
			},
		},
	})

	// Grant the user read_access
	err = user_manager.AddUserToOrg(self.Ctx, services.AddNewUser,
		utils.GetSuperuserName(self.ConfigObj),
		reader_username, []string{"root"}, &acls_proto.ApiClientACL{
			ReadResults: true,
		})
	assert.NoError(self.T(), err)

	writer_username := "WriterUser"
	bundle, err = crypto.GenerateServerCert(self.ConfigObj, writer_username)
	assert.NoError(self.T(), err)

	// Build a mocked context to emulate the one the grpc server builds.
	writer_ctx := peer.NewContext(self.Ctx, &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{
					bundle.Certificate,
				},
			},
		},
	})

	// The writer gets DATASTORE_ACCESS
	err = user_manager.AddUserToOrg(self.Ctx, services.AddNewUser,
		utils.GetSuperuserName(self.ConfigObj),
		writer_username, []string{"root"}, &acls_proto.ApiClientACL{
			DatastoreAccess: true,
		})
	assert.NoError(self.T(), err)

	api_server := &api.ApiServer{}

	client_subject := &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: []string{
				"clients", "C.1234",
			},
		},
		Data: []byte(`{"client_id":"C.1234"}`),
	}

	// Reader can not access the raw datastore at all - this requires
	// DATASTORE_ACCESS permission
	_, err = api_server.SetSubject(reader_ctx, client_subject)
	assert.Error(self.T(), err)

	_, err = api_server.GetSubject(reader_ctx, client_subject)
	assert.Error(self.T(), err)

	_, err = api_server.ListChildren(reader_ctx, client_subject)
	assert.Error(self.T(), err)

	_, err = api_server.DeleteSubject(reader_ctx, client_subject)
	assert.Error(self.T(), err)

	// But the writer can do anything
	_, err = api_server.SetSubject(writer_ctx, client_subject)
	assert.NoError(self.T(), err)

	res, err := api_server.GetSubject(writer_ctx, client_subject)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), client_subject.Data, res.Data)

	_, err = api_server.ListChildren(writer_ctx, client_subject)
	assert.NoError(self.T(), err)

	_, err = api_server.DeleteSubject(writer_ctx, client_subject)
	assert.NoError(self.T(), err)

	// A location in a public area
	req := &api_proto.VFSFileBuffer{
		Data:       []byte("hello"),
		Components: []string{"clients", "C.1234", "foobar"},
		PathType:   int64(file_store_api.PATH_TYPE_FILESTORE_JSON),
		OrgId:      "root",
		Length:     10,
	}

	// A location in a private area
	private_req := &api_proto.VFSFileBuffer{
		Components: []string{"config", "foobar"},
		PathType:   int64(file_store_api.PATH_TYPE_FILESTORE_JSON),
		OrgId:      "root",
		Length:     10,
	}

	// reader can not write to filestore
	_, err = api_server.FSWriteBuffer(reader_ctx, req)
	assert.Error(self.T(), err)
	assert.ErrorContains(self.T(), err, "User is not allowed to access datastore")

	// writer can write to filestore
	fs_res, err := api_server.FSWriteBuffer(writer_ctx, req)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), int64(5), fs_res.Written)

	// Can also write in private area
	_, err = api_server.FSWriteBuffer(writer_ctx, private_req)
	assert.NoError(self.T(), err)

	// Reader can read it again.
	fsget_buf_res, err := api_server.VFSGetBuffer(reader_ctx, req)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), req.Data, fsget_buf_res.Data)

	// Reader can not read private data
	fsget_buf_res, err = api_server.VFSGetBuffer(reader_ctx, private_req)
	assert.Error(self.T(), err)
	assert.ErrorContains(self.T(), err, "No access to filesystem path")

	// But the writer has unfettered access
	fsget_buf_res, err = api_server.VFSGetBuffer(writer_ctx, private_req)
	assert.NoError(self.T(), err)

	// Reader can stat a file
	data_req := &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: req.Components,
			PathType:   req.PathType,
		},
	}

	private_data_req := &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: private_req.Components,
			PathType:   private_req.PathType,
		},
	}

	// reader can stat the public data
	fsstat_buf_res, err := api_server.FSStat(reader_ctx, data_req)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(data_req.Pathspec.Components), len(fsstat_buf_res.Components))

	// But not the private data
	_, err = api_server.FSStat(reader_ctx, private_data_req)
	assert.ErrorContains(self.T(), err, "No access to filesystem path")

	// But the writer can stat anything
	_, err = api_server.FSStat(writer_ctx, private_data_req)
	assert.NoError(self.T(), err)

	// Reader can read directories
	readdir_req := &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: []string{"clients"},
			PathType:   req.PathType,
		},
	}

	private_readdir_req := &api_proto.DataRequest{
		Pathspec: &api_proto.DSPathSpec{
			Components: []string{"config"},
			PathType:   req.PathType,
		},
	}

	//  Can read directories in public space
	dir_res, err := api_server.FSReadDir(reader_ctx, readdir_req)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(dir_res.Items))

	// But can not read dir in private spaces
	_, err = api_server.FSReadDir(reader_ctx, private_readdir_req)
	assert.Error(self.T(), err)
	assert.ErrorContains(self.T(), err, "No access to filesystem path")

	// writer can read dir in private spaces
	_, err = api_server.FSReadDir(writer_ctx, private_readdir_req)
	assert.NoError(self.T(), err)

	// Reader can not delete
	_, err = api_server.FSDelete(reader_ctx, data_req)
	assert.Error(self.T(), err)

	// Writer can delete
	_, err = api_server.FSDelete(writer_ctx, data_req)
	assert.NoError(self.T(), err)

	_, err = api_server.FSDelete(writer_ctx, private_data_req)
	assert.NoError(self.T(), err)
}

func TestAPIDatastore(t *testing.T) {
	suite.Run(t, &DatastoreAPITest{})
}
