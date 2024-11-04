package http_comms_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"

	// For clock()
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/common"
)

type TestSuite struct {
	test_utils.TestSuite

	client_id string
	port      int

	client_url string
}

func (self *TestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.Interrogation = true
	self.ConfigObj.Services.Launcher = true
	self.ConfigObj.Services.UserManager = true
	self.ConfigObj.Services.ClientMonitoring = true
	self.ConfigObj.Services.HuntDispatcher = true

	self.LoadArtifactsIntoConfig([]string{`
name: TestArtifact
sources:
- query: SELECT log(message="Hello") AS Hello FROM scope()
`})

	var err error
	self.port, err = vtesting.GetFreePort()
	assert.NoError(self.T(), err)
	self.client_url = fmt.Sprintf("http://localhost:%d/", self.port)

	self.TestSuite.SetupTest()

	self.EnrolClient()

	self.ConfigObj.Client.MaxPoll = 1
	self.ConfigObj.Client.MaxPollStd = 1
}

// Create a client record so server and client can talk.
func (self *TestSuite) EnrolClient() {
	private_key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr(
		[]byte(self.ConfigObj.Writeback.PrivateKey))
	assert.NoError(self.T(), err)

	pem := &crypto_proto.PublicKey{
		Pem:        crypto_utils.PublicKeyToPem(&private_key.PublicKey),
		EnrollTime: 1000,
	}

	self.client_id = crypto_utils.ClientIDFromPublicKey(&private_key.PublicKey)
	client_path_manager := paths.NewClientPathManager(self.client_id)
	db, _ := datastore.GetDB(self.ConfigObj)

	// Write a client record.
	client_info_obj := &actions_proto.ClientInfo{
		ClientId: self.client_id,
	}
	err = db.SetSubject(self.ConfigObj, client_path_manager.Path(), client_info_obj)
	assert.NoError(self.T(), err)

	err = db.SetSubject(self.ConfigObj, client_path_manager.Key(), pem)
	assert.NoError(self.T(), err)
}

// Create a server
func (self *TestSuite) makeServer(
	server_ctx context.Context,
	server_wg *sync.WaitGroup) {

	// Create a new server
	server_obj, err := server.NewServer(server_ctx, self.ConfigObj, server_wg)
	assert.NoError(self.T(), err)

	mux := http.NewServeMux()
	server.PrepareFrontendMux(self.ConfigObj, server_obj, mux)

	err = api.StartFrontendPlainHttp(server_ctx, server_wg, self.ConfigObj, server_obj, mux)
	assert.NoError(self.T(), err)

	// Wait for it to come up
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		url := fmt.Sprintf("http://localhost:%d/server.pem", self.port)
		req, err := http.Get(url)
		if err != nil || req.StatusCode != http.StatusOK {
			return false
		}
		defer req.Body.Close()

		return true
	})
}

// Create a client
func (self *TestSuite) makeClient(
	client_ctx context.Context,
	client_wg *sync.WaitGroup) *http_comms.HTTPCommunicator {
	ctx := context.Background()
	manager, err := crypto_client.NewClientCryptoManager(ctx,
		self.ConfigObj, []byte(self.ConfigObj.Writeback.PrivateKey))
	assert.NoError(self.T(), err)

	exe, err := executor.NewClientExecutor(
		client_ctx, manager.ClientId(), self.ConfigObj)
	assert.NoError(self.T(), err)

	on_error := func() {}
	comm, err := http_comms.NewHTTPCommunicator(
		client_ctx,
		self.ConfigObj,
		manager,
		exe,
		[]string{self.client_url},
		on_error, utils.RealClock{},
	)
	assert.NoError(self.T(), err)

	client_wg.Add(1)
	go func() {
		defer client_wg.Done()

		comm.Run(client_ctx, client_wg)
	}()

	return comm
}

// Start a collection, and see if it works.
func (self *TestSuite) TestScheduleCollection() {
	self.client_url = fmt.Sprintf("http://localhost:%d/", self.port)
	closer := self.testScheduleCollection()
	defer closer()
}

func (self *TestSuite) TestScheduleCollectionWithWebSocket() {
	self.client_url = fmt.Sprintf("ws://localhost:%d/", self.port)
	self.ConfigObj.Client.WsPingWaitSec = 1

	closer := self.testScheduleCollection()
	defer closer()

	time.Sleep(3 * time.Second)

	// Make sure the server sent some pings to the client
	pings := []string{}
	for _, l := range logging.GetMemoryLogs() {
		if strings.Contains(l, "Ping") {
			pings = append(pings, l)
		}
	}
	assert.True(self.T(), len(pings) > 5)
}

func (self *TestSuite) testScheduleCollection() (closer func()) {
	self.ConfigObj.Frontend.BindPort = uint32(self.port)
	self.ConfigObj.Client.ServerUrls = []string{self.client_url}

	server_ctx, server_cancel := context.WithCancel(self.Ctx)
	server_wg := &sync.WaitGroup{}

	self.makeServer(server_ctx, server_wg)

	client_ctx, client_cancel := context.WithCancel(self.Ctx)
	client_wg := &sync.WaitGroup{}

	closer = func() {
		client_cancel()
		server_cancel()

		server_wg.Wait()
		client_wg.Wait()
	}

	comm := self.makeClient(client_ctx, client_wg)

	// Make sure the client is properly enrolled
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		err := comm.Sender.SendToURL(client_ctx, [][]byte{},
			!http_comms.URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)
		assert.NoError(self.T(), err)

		return vtesting.ContainsString("response with status: 200",
			logging.GetMemoryLogs())
	})

	// Schdule a collection on the client
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Artifacts: []string{"TestArtifact"},
		Creator:   utils.GetSuperuserName(self.ConfigObj),
	}

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		request, nil)
	assert.NoError(self.T(), err)

	var flow *api_proto.FlowDetails

	vtesting.WaitUntil(20*time.Second, self.T(), func() bool {
		flow, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.client_id, flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.State == flows_proto.ArtifactCollectorContext_FINISHED
	})
	assert.Equal(self.T(), uint64(5), flow.Context.TotalLogs)
	assert.Equal(self.T(), uint64(1), flow.Context.TotalCollectedRows)
	assert.Equal(self.T(), "TestArtifact", flow.Context.ArtifactsWithResults[0])

	return closer
}

func (self *TestSuite) TestServerRotateKeyE2E() {
	logging.ClearMemoryLogs()

	self.ConfigObj.Frontend.BindPort = uint32(self.port)
	self.ConfigObj.Client.ServerUrls = []string{
		fmt.Sprintf("http://localhost:%d", self.port),
	}

	server_ctx, server_cancel := context.WithCancel(self.Ctx)
	server_wg := &sync.WaitGroup{}

	self.makeServer(server_ctx, server_wg)

	client_ctx, client_cancel := context.WithCancel(self.Ctx)
	client_wg := &sync.WaitGroup{}

	comm := self.makeClient(client_ctx, client_wg)

	// Make sure the client is properly enrolled
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		err := comm.Sender.SendToURL(client_ctx, [][]byte{},
			!http_comms.URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)
		assert.NoError(self.T(), err)

		return vtesting.ContainsString("response with status: 200",
			logging.GetMemoryLogs())
	})

	// Stop the receive and send loops to prevent race with direct post
	comm.SetPause(true)

	logging.ClearMemoryLogs()

	// Now rotate the server keys: First shut down the old server.
	server_cancel()
	server_wg.Wait()

	// Now rekey the server
	frontend_cert, err := crypto.GenerateServerCert(
		self.ConfigObj, utils.GetSuperuserName(self.ConfigObj))
	assert.NoError(self.T(), err)

	self.ConfigObj.Frontend.Certificate = frontend_cert.Cert
	self.ConfigObj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Now bring up the new server.
	server_ctx, server_cancel = context.WithCancel(self.Ctx)
	server_wg = &sync.WaitGroup{}

	logging.ClearMemoryLogs()

	self.makeServer(server_ctx, server_wg)

	// Make sure the client properly rekeys and continues to talk to the server
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		err := comm.Sender.SendToURL(client_ctx, [][]byte{},
			!http_comms.URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)
		if err != nil {
			return false
		}

		// Make sure the client rekeys and connects successfully.
		return vtesting.ContainsString("response with status: 200",
			logging.GetMemoryLogs()) &&
			vtesting.ContainsString("Received PEM for VelociraptorServer",
				logging.GetMemoryLogs())
	})

	// Done
	server_cancel()
	client_cancel()

	// Wait for the server to quit.
	client_wg.Wait()
	server_wg.Wait()
}

func TestClientServerComms(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
