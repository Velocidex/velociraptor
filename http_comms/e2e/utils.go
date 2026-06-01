package e2e

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/http_comms"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

// Adds test suite for end to end testing

type E2ETestSuite struct {
	test_utils.TestSuite

	ClientId string
	Port     int

	ClientUrl string

	Artifacts []string
}

func (self *E2ETestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.Interrogation = true
	self.ConfigObj.Services.Launcher = true
	self.ConfigObj.Services.UserManager = true
	self.ConfigObj.Services.ClientMonitoring = true
	self.ConfigObj.Services.HuntDispatcher = true

	self.LoadArtifactsIntoConfig(self.Artifacts)

	var err error
	self.Port, err = vtesting.GetFreePort()
	assert.NoError(self.T(), err)
	self.ClientUrl = fmt.Sprintf("http://localhost:%d/", self.Port)

	self.TestSuite.SetupTest()

	self.EnrolClient()

	self.ConfigObj.Client.MaxPoll = 1
	self.ConfigObj.Client.MaxPollStd = 1
}

func (self *E2ETestSuite) StartServerAndClient() (closer func()) {
	self.ConfigObj.Frontend.BindPort = uint32(self.Port)
	self.ConfigObj.Client.ServerUrls = []string{self.ClientUrl}

	server_ctx, server_cancel := context.WithCancel(self.Ctx)
	server_wg := &sync.WaitGroup{}

	self.MakeServer(server_ctx, server_wg)

	client_ctx, client_cancel := context.WithCancel(self.Ctx)
	client_wg := &sync.WaitGroup{}

	closer = func() {
		client_cancel()
		server_cancel()

		server_wg.Wait()
		client_wg.Wait()
	}

	comm := self.MakeClient(client_ctx, client_wg)

	// Make sure the client is properly enrolled
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		err := comm.Sender.SendToURL(client_ctx, [][]byte{},
			!http_comms.URGENT, crypto_proto.PackedMessageList_ZCOMPRESSION)
		assert.NoError(self.T(), err)

		return vtesting.ContainsString("response with status: 200",
			logging.GetMemoryLogs())
	})

	return closer
}

// Create a client record so server and client can talk.
func (self *E2ETestSuite) EnrolClient() {
	private_key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr(
		[]byte(self.ConfigObj.Writeback.PrivateKey))
	assert.NoError(self.T(), err)

	pem := &crypto_proto.PublicKey{
		Pem:        crypto_utils.PublicKeyToPem(&private_key.PublicKey),
		EnrollTime: 1000,
	}

	self.ClientId = crypto_utils.ClientIDFromPublicKey(&private_key.PublicKey)
	client_path_manager := paths.NewClientPathManager(self.ClientId)
	db, _ := datastore.GetDB(self.ConfigObj)

	// Write a client record.
	client_info_obj := &actions_proto.ClientInfo{
		ClientId: self.ClientId,
	}
	err = db.SetSubject(self.ConfigObj, client_path_manager.Path(), client_info_obj)
	assert.NoError(self.T(), err)

	err = db.SetSubject(self.ConfigObj, client_path_manager.Key(), pem)
	assert.NoError(self.T(), err)
}

// Create a server
func (self *E2ETestSuite) MakeServer(
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
		url := fmt.Sprintf("http://localhost:%d/server.pem", self.Port)
		req, err := http.Get(url)
		if err != nil || req.StatusCode != http.StatusOK {
			return false
		}
		defer req.Body.Close()

		return true
	})
}

// Create a client
func (self *E2ETestSuite) MakeClient(
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
		[]string{self.ClientUrl},
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
