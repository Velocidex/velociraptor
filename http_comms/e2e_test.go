package http_comms

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/executor"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_info"
	"www.velocidex.com/golang/velociraptor/services/interrogation"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type TestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	sm         *services.Service
}

func (self *TestSuite) SetupTest() {
	t := self.T()

	config_obj, err := new(config.Loader).WithFileLoader(
		"../http_comms/test_data/server.config.yaml").
		WithRequiredClient().WithWriteback().LoadAndValidate()
	assert.NoError(t, err)

	self.config_obj = config_obj

	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	self.config_obj.Frontend.IsMaster = true

	// Start the journaling service manually for tests.
	require.NoError(t, self.sm.Start(journal.StartJournalService))
	require.NoError(t, self.sm.Start(notifications.StartNotificationService))
	require.NoError(t, self.sm.Start(interrogation.StartInterrogationService))
	require.NoError(t, self.sm.Start(repository.StartRepositoryManager))
	require.NoError(t, self.sm.Start(launcher.StartLauncherService))
	require.NoError(t, self.sm.Start(client_info.StartClientInfoService))

	self.EnrolClient()

	self.config_obj.Client.MaxPoll = 1
	self.config_obj.Client.MaxPollStd = 1
}

func (self *TestSuite) TearDownTest() {
	self.sm.Close()

	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
}

// Create a client record so server and client can talk.
func (self *TestSuite) EnrolClient() {
	private_key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr(
		[]byte(self.config_obj.Writeback.PrivateKey))
	assert.NoError(self.T(), err)

	pem := &crypto_proto.PublicKey{
		Pem:        crypto_utils.PublicKeyToPem(&private_key.PublicKey),
		EnrollTime: 1000,
	}

	self.client_id = crypto_utils.ClientIDFromPublicKey(&private_key.PublicKey)
	client_path_manager := paths.NewClientPathManager(self.client_id)
	db, _ := datastore.GetDB(self.config_obj)

	// Write a client record.
	client_info_obj := &actions_proto.ClientInfo{
		ClientId: self.client_id,
	}
	err = db.SetSubject(self.config_obj, client_path_manager.Path(), client_info_obj)
	assert.NoError(self.T(), err)

	err = db.SetSubject(self.config_obj, client_path_manager.Key().Path(), pem)
	assert.NoError(self.T(), err)
}

// Create a server
func (self *TestSuite) makeServer(
	server_ctx context.Context,
	server_wg *sync.WaitGroup) {

	// Create a new server
	server_obj, err := server.NewServer(server_ctx, self.config_obj, server_wg)
	assert.NoError(self.T(), err)

	mux := http.NewServeMux()
	server.PrepareFrontendMux(self.config_obj, server_obj, mux)

	err = api.StartFrontendPlainHttp(server_ctx, server_wg, self.config_obj, server_obj, mux)
	assert.NoError(self.T(), err)

	// Wait for it to come up
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		req, err := http.Get("http://localhost:8000/server.pem")
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
	client_wg *sync.WaitGroup) *HTTPCommunicator {
	manager, err := crypto_client.NewClientCryptoManager(
		self.config_obj, []byte(self.config_obj.Writeback.PrivateKey))
	assert.NoError(self.T(), err)

	exe, err := executor.NewClientExecutor(client_ctx, self.config_obj)
	assert.NoError(self.T(), err)

	on_error := func() {}
	comm, err := NewHTTPCommunicator(
		self.config_obj,
		manager,
		exe,
		[]string{"http://localhost:8000/"},
		on_error, utils.RealClock{},
	)
	assert.NoError(self.T(), err)

	client_wg.Add(1)
	go func() {
		defer client_wg.Done()

		comm.Run(client_ctx)
	}()

	return comm
}

func (self *TestSuite) TestServerRotateKeyE2E() {
	server_ctx, server_cancel := context.WithCancel(self.sm.Ctx)
	server_wg := &sync.WaitGroup{}

	self.makeServer(server_ctx, server_wg)

	client_ctx, client_cancel := context.WithCancel(self.sm.Ctx)
	client_wg := &sync.WaitGroup{}

	comm := self.makeClient(client_ctx, client_wg)
	err := comm.sender.sendToURL(client_ctx, [][]byte{}, false)
	assert.NoError(self.T(), err)

	// Make sure the client is properly enrolled
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		// json.Dump(logging.GetMemoryLogs())
		return vtesting.ContainsString("response with status: 200", logging.GetMemoryLogs())
	})
	//	json.Dump(logging.GetMemoryLogs())
	logging.ClearMemoryLogs()

	// Now rotate the server keys: First shut down the old server.
	server_cancel()
	server_wg.Wait()

	// Now rekey the server
	frontend_cert, err := crypto.GenerateServerCert(
		self.config_obj, self.config_obj.Client.PinnedServerName)
	assert.NoError(self.T(), err)

	self.config_obj.Frontend.Certificate = frontend_cert.Cert
	self.config_obj.Frontend.PrivateKey = frontend_cert.PrivateKey

	// Now bring up the new server.
	server_ctx, server_cancel = context.WithCancel(self.sm.Ctx)
	server_wg = &sync.WaitGroup{}

	self.makeServer(server_ctx, server_wg)

	//	json.Dump(logging.GetMemoryLogs())

	logging.ClearMemoryLogs()

	// Sending another one will produce an error.
	err = comm.sender.sendToURL(client_ctx, [][]byte{}, false)
	assert.Error(self.T(), err)

	// Make sure the client properly rekeys and continues to talk to the server
	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// json.Dump(logging.GetMemoryLogs())
		return vtesting.ContainsString("response with status: 200", logging.GetMemoryLogs())
	})

	// Done
	server_cancel()
	client_cancel()

	// Wait for the server to quit.
	client_wg.Wait()
	server_wg.Wait()
}

func TestClientServerComms(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	config_obj.Datastore.Implementation = "Test"

	suite.Run(t, &TestSuite{
		config_obj: config_obj,
	})
}
