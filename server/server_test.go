package server_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/clients"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	file_store_api "www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/client_monitoring"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/services/interrogation"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"

	_ "www.velocidex.com/golang/velociraptor/result_sets/simple"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

var (
	mock_definitions = []string{`
name: Windows.Remediation.QuarantineMonitor
type: SERVER_EVENT
`, `
name: System.Hunt.Creation
type: SERVER_EVENT
`, `
name: System.Flow.Archive
type: SERVER
`, `
name: Server.Internal.Enrollment
type: INTERNAL
`, `
name: Generic.Client.Info
type: CLIENT
sources:
- name: BasicInformation
  query: SELECT * FROM info()
- name: Users
  precondition: SELECT OS From info() where OS = 'windows'
  query: SELECT * FROM info()
`}
)

type ServerTestSuite struct {
	test_utils.TestSuite
	server        *server.Server
	client_crypto *crypto_client.ClientCryptoManager
	client_id     string
}

type MockAPIClientFactory struct {
	mock api_proto.APIClient
}

func (self MockAPIClientFactory) GetAPIClient(
	ctx context.Context,
	config_obj *config_proto.Config) (api_proto.APIClient, func() error, error) {
	return self.mock, func() error { return nil }, nil
}

func (self *ServerTestSuite) SetupTest() {
	var err error
	self.TestSuite.SetupTest()
	self.LoadArtifacts(mock_definitions)

	require.NoError(self.T(), self.Sm.Start(client_monitoring.StartClientMonitoringService))
	require.NoError(self.T(), self.Sm.Start(hunt_dispatcher.StartHuntDispatcher))
	require.NoError(self.T(), self.Sm.Start(interrogation.StartInterrogationService))

	self.server, err = server.NewServer(self.Sm.Ctx, self.ConfigObj, self.Sm.Wg)
	require.NoError(self.T(), err)

	self.client_crypto, err = crypto_client.NewClientCryptoManager(
		self.ConfigObj, []byte(self.ConfigObj.Writeback.PrivateKey))
	require.NoError(self.T(), err)

	_, err = self.client_crypto.AddCertificate([]byte(
		self.ConfigObj.Frontend.Certificate))

	require.NoError(self.T(), err)

	self.client_id = self.client_crypto.ClientId
}

func (self *ServerTestSuite) TestEnrollment() {
	// Enrollment occurs when the client sends an unauthenticated
	// CSR message.
	csr_message, err := self.client_crypto.GetCSR()
	require.NoError(self.T(), err)

	wg := &sync.WaitGroup{}
	messages := []*ordereddict.Dict{}

	wg.Add(1)
	services.GetPublishedEvents(
		self.ConfigObj, "Server.Internal.Enrollment", wg, 1, &messages)

	self.server.ProcessSingleUnauthenticatedMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			CSR: &crypto_proto.Certificate{Pem: csr_message}})

	db, err := datastore.GetDB(self.ConfigObj)
	require.NoError(self.T(), err)

	pub_key := &crypto_proto.PublicKey{}
	err = db.GetSubject(
		self.ConfigObj,
		paths.NewClientPathManager(self.client_id).Key(),
		pub_key)

	assert.NoError(self.T(), err)

	assert.Regexp(self.T(), "RSA PUBLIC KEY", string(pub_key.Pem))

	wg.Wait()

	// Check that Server.Internal.Enrollment is scheduled for this
	// client. The entrollment service will respond to this
	// message.
	client_id, pres := messages[0].GetString("ClientId")
	assert.True(self.T(), pres)
	assert.Equal(self.T(), client_id, self.client_id)
}

func (self *ServerTestSuite) TestClientEventTable() {
	t := self.T()

	ctrl := gomock.NewController(self.T())
	defer ctrl.Finish()

	runner := flows.NewFlowRunner(self.ConfigObj)
	defer runner.Close()

	// Set a new event monitoring table
	err := services.ClientEventManager().SetClientMonitoringState(
		context.Background(), self.ConfigObj, "",
		&flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"Generic.Client.Stats"},
			},
		})
	require.NoError(t, err)

	// The version of the currently installed table.
	version := services.ClientEventManager().GetClientMonitoringState().Version

	// Wait a bit.
	time.Sleep(time.Second)

	// Send a foreman checkin message from client with old event
	// table version.
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source: self.client_id,
			ForemanCheckin: &actions_proto.ForemanCheckin{
				LastEventTableVersion: 0,
			},
		})

	tasks, err := clients.GetClientTasks(self.ConfigObj,
		self.client_id, true /* do_not_lease */)
	assert.NoError(t, err)
	assert.Equal(t, len(tasks), 1)

	// This should send an UpdateEventTable message.
	assert.Equal(t, tasks[0].SessionId, "F.Monitoring")
	assert.NotNil(t, tasks[0].UpdateEventTable)

	// The client version is more advanced than the server version
	// therefore no new updates required.
	assert.True(t, tasks[0].UpdateEventTable.Version > version)
}

// Create a new hunt. Client sends a ForemanCheckin message with
// LastHuntTimestamp = 0 and will receive the UpdateForeman message.
func (self *ServerTestSuite) TestForeman() {
	t := self.T()
	runner := flows.NewFlowRunner(self.ConfigObj)
	defer runner.Close()

	db, err := datastore.GetDB(self.ConfigObj)
	require.NoError(self.T(), err)

	// The hunt will launch the Generic.Client.Info on the client.
	expected := api.MakeCollectorRequest(
		self.client_id, "Generic.Client.Info")

	hunt_id, err := flows.CreateHunt(
		context.Background(), self.ConfigObj,
		vql_subsystem.NullACLManager{},
		&api_proto.Hunt{
			State:        api_proto.Hunt_RUNNING,
			StartRequest: expected,
		})
	assert.NoError(t, err)

	// Check for hunt object in the data store.
	hunt := &api_proto.Hunt{}
	hunt_path_manager := paths.NewHuntPathManager(hunt_id)
	err = db.GetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt)
	require.NoError(t, err)

	assert.NotNil(t, hunt.StartRequest.CompiledCollectorArgs)

	hunt.StartRequest.CompiledCollectorArgs = nil
	expected.CompiledCollectorArgs = nil

	assert.Equal(t, hunt.StartRequest, expected)

	// Send a foreman checkin message from client with old hunt
	// timestamp.
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source: self.client_id,
			ForemanCheckin: &actions_proto.ForemanCheckin{
				LastHuntTimestamp: 0,

				// We do not want to trigger an event
				// table update in this test so we
				// pretend our version is later than
				// the automatic table that will be
				// created.
				LastEventTableVersion: 10000000000000000000,
			},
		})

	// Server should schedule the new hunt on the client.
	tasks, err := clients.GetClientTasks(self.ConfigObj,
		self.client_id, true /* do_not_lease */)
	assert.NoError(t, err)
	assert.Equal(t, len(tasks), 1)

	// Task should be UpdateForeman message.
	assert.Equal(t, tasks[0].SessionId, "F.Monitoring")
	require.NotNil(t, tasks[0].UpdateForeman)
	assert.Equal(t, tasks[0].UpdateForeman.LastHuntTimestamp, services.GetHuntDispatcher().
		GetLastTimestamp())
}

func (self *ServerTestSuite) RequiredFilestoreContains(
	filename file_store_api.FSPathSpec, regex string) {

	file_store_factory := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)

	value, pres := file_store_factory.Get(filename.AsFilestoreFilename(
		self.ConfigObj))
	if !pres {
		self.T().FailNow()
	}

	require.Regexp(self.T(), regexp.MustCompile(regex), string(value))
}

// Receiving a response from the server to the monitoring flow will
// write the rows into a csv file in the client's monitoring area.
func (self *ServerTestSuite) TestMonitoring() {
	runner := flows.NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: constants.MONITORING_WELL_KNOWN_FLOW,
			VQLResponse: &actions_proto.VQLResponse{
				Columns: []string{
					"ClientId", "Timestamp", "Fqdn", "HuntId"},
				Response: fmt.Sprintf(
					`[{"ClientId": "%s", "HuntId": "H.123"}]`,
					self.client_id),
				Query: &actions_proto.VQLRequest{
					Name: "Generic.Client.Stats",
				},
			},
		})
	runner.Close()

	path_manager, err := artifacts.NewArtifactPathManager(self.ConfigObj,
		self.client_id, constants.MONITORING_WELL_KNOWN_FLOW,
		"Generic.Client.Stats")
	assert.NoError(self.T(), err)

	self.RequiredFilestoreContains(path_manager.Path(), self.client_id)
}

// Monitoring queries which upload data.
func (self *ServerTestSuite) TestMonitoringWithUpload() {
	runner := flows.NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: constants.MONITORING_WELL_KNOWN_FLOW,
			RequestId: constants.TransferWellKnownFlowId,
			FileBuffer: &actions_proto.FileBuffer{
				Pathspec: &actions_proto.PathSpec{
					Path: "/etc/passwd",
				},
				Data: []byte("Hello"),
				Size: 10000,
			},
		})
	runner.Close()

	path_manager := paths.NewFlowPathManager(
		self.client_id, "F.Monitoring").GetUploadsFile("file", "/etc/passwd")
	self.RequiredFilestoreContains(path_manager.Path(), "Hello")
}

// Test that log messages are written to the flow
func (self *ServerTestSuite) TestLog() {
	t := self.T()

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	// Emulate log messages from client to flow delivered in
	// separate POST.
	runner := flows.NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			LogMessage: &crypto_proto.LogMessage{
				Message: "Foobar",
			},
		})
	runner.Close()

	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			LogMessage: &crypto_proto.LogMessage{
				Message: "ZooBar",
			},
		})
	runner.Close()

	path_spec := paths.NewFlowPathManager(self.client_id, flow_id).Log()
	self.RequiredFilestoreContains(path_spec, "Foobar")
	self.RequiredFilestoreContains(path_spec, "ZooBar")
}

// Test that messages intended to unknown flows are handled
// gracefully.
func (self *ServerTestSuite) TestLogToUnknownFlow() {
	// Emulate a log message from client to flow.
	runner := flows.NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: "F.1234",
			LogMessage: &crypto_proto.LogMessage{
				Message: "Foobar",
			},
		})
	runner.Close()

	t := self.T()

	// Cancellation message should never be sent due to log.
	tasks, err := clients.GetClientTasks(self.ConfigObj,
		self.client_id, true /* do_not_lease */)
	assert.NoError(t, err)
	assert.Equal(t, len(tasks), 0)

	runner = flows.NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: "F.1234",
			Status:    &crypto_proto.GrrStatus{},
		})
	runner.Close()

	// Cancellation message should never be sent due to status.
	tasks, err = clients.GetClientTasks(self.ConfigObj,
		self.client_id, true /* do_not_lease */)
	assert.NoError(t, err)
	assert.Equal(t, len(tasks), 0)

	runner = flows.NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:      self.client_id,
			SessionId:   "F.1234",
			VQLResponse: &actions_proto.VQLResponse{},
		})
	runner.Close()

	// Cancellation message should be sent due to response
	// messages.
	tasks, err = clients.GetClientTasks(self.ConfigObj,
		self.client_id, true /* do_not_lease */)
	assert.NoError(t, err)
	assert.Equal(t, len(tasks), 1)
}

func (self *ServerTestSuite) TestScheduleCollection() {
	t := self.T()
	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Artifacts: []string{"Generic.Client.Info"},
	}

	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(t, err)

	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(
		context.Background(),
		self.ConfigObj,
		vql_subsystem.NullACLManager{},
		repository,
		request, nil)

	db, err := datastore.GetDB(self.ConfigObj)
	require.NoError(t, err)

	// Launching the artifact will schedule one query on the client.
	tasks, err := clients.GetClientTasks(
		self.ConfigObj, self.client_id,
		true /* do_not_lease */)
	assert.NoError(t, err)
	assert.Equal(t, len(tasks), 2)

	collection_context := &flows_proto.ArtifactCollectorContext{}
	path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	err = db.GetSubject(self.ConfigObj, path_manager.Path(), collection_context)
	require.NoError(t, err)

	assert.Equal(t, collection_context.Request, request)
}

// Schedule a flow in the database and return its flow id
func (self *ServerTestSuite) createArtifactCollection() (string, error) {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(self.T(), err)

	// Schedule a flow in the database.
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(
		context.Background(),
		self.ConfigObj,
		vql_subsystem.NullACLManager{},
		repository,
		&flows_proto.ArtifactCollectorArgs{
			ClientId:  self.client_id,
			Artifacts: []string{"Generic.Client.Info"},
		}, nil)

	return flow_id, err
}

// Test that uploaded buffers are written to the file store.
func (self *ServerTestSuite) TestUploadBuffer() {
	t := self.T()

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	// Emulate a response from this flow.
	runner := flows.NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.TransferWellKnownFlowId,
			FileBuffer: &actions_proto.FileBuffer{
				Pathspec: &actions_proto.PathSpec{
					Path:     "/tmp/foobar",
					Accessor: "file",
				},
				Offset: 0,
				Data:   []byte("hello world"),
				Size:   11,
			},
		})
	runner.Close()

	flow_path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	self.RequiredFilestoreContains(
		flow_path_manager.GetUploadsFile("file", "/tmp/foobar").Path(),
		"hello world")

	self.RequiredFilestoreContains(
		flow_path_manager.UploadMetadata(), flow_id)
}

// Test VQLResponse are written correctly.
func (self *ServerTestSuite) TestVQLResponse() {
	t := self.T()

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	// Emulate a response from this flow.
	runner := flows.NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.ProcessVQLResponses,
			VQLResponse: &actions_proto.VQLResponse{
				Columns: []string{"ClientId", "Column1"},
				Response: fmt.Sprintf(
					`[{"ClientId": "%s", "Column1": "Foo"}]`,
					self.client_id),
				Query: &actions_proto.VQLRequest{
					Name: "Generic.Client.Info",
				},
			},
		})
	runner.Close()

	flow_path_manager, err := artifacts.NewArtifactPathManager(self.ConfigObj,
		self.client_id, flow_id, "Generic.Client.Info")
	assert.NoError(self.T(), err)

	self.RequiredFilestoreContains(flow_path_manager.Path(), self.client_id)
}

// Errors from the client kill the flow.
func (self *ServerTestSuite) TestErrorMessage() {
	t := self.T()

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	// Emulate a response from this flow.
	runner := flows.NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.ProcessVQLResponses,
			Status: &crypto_proto.GrrStatus{
				Status:       crypto_proto.GrrStatus_GENERIC_ERROR,
				ErrorMessage: "Error generated.",
				Backtrace:    "I am a backtrace",
			},
		})
	runner.Close()

	db, _ := datastore.GetDB(self.ConfigObj)

	// A log is generated
	path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	self.RequiredFilestoreContains(path_manager.Log(), "Error generated")

	// The collection_context is marked as errored.
	collection_context := &flows_proto.ArtifactCollectorContext{}
	err = db.GetSubject(self.ConfigObj, path_manager.Path(),
		collection_context)
	require.NoError(t, err)

	require.Regexp(self.T(), regexp.MustCompile("Error generated"),
		collection_context.Status)

	require.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		collection_context.State)
}

// Successful status should terminate the flow.
func (self *ServerTestSuite) TestCompletions() {
	t := self.T()

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	// Emulate a response from this flow.
	runner := flows.NewFlowRunner(self.ConfigObj)

	// Generic.Client.Info sends two requests, lets complete them both.
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.ProcessVQLResponses,
			Status: &crypto_proto.GrrStatus{
				Status: crypto_proto.GrrStatus_OK,
			},
		})
	runner.Close()

	db, _ := datastore.GetDB(self.ConfigObj)

	// The collection_context is marked as errored.
	collection_context := &flows_proto.ArtifactCollectorContext{}
	path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	err = db.GetSubject(self.ConfigObj, path_manager.Path(), collection_context)
	require.NoError(t, err)

	// Flow not complete yet - still an outstanding request.
	require.Equal(self.T(), flows_proto.ArtifactCollectorContext_RUNNING,
		collection_context.State)

	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.ProcessVQLResponses,
			Status: &crypto_proto.GrrStatus{
				Status: crypto_proto.GrrStatus_OK,
			},
		})
	runner.Close()

	// Flow should be complete now that second response arrived.
	err = db.GetSubject(self.ConfigObj, path_manager.Path(), collection_context)
	require.NoError(t, err)

	require.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED,
		collection_context.State)

}

// Test flow cancellation
func (self *ServerTestSuite) TestCancellation() {
	ctrl := gomock.NewController(self.T())
	defer ctrl.Finish()

	t := self.T()

	db, err := datastore.GetDB(self.ConfigObj)
	require.NoError(t, err)

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	// One task is scheduled for the client.
	tasks, err := clients.GetClientTasks(self.ConfigObj,
		self.client_id, true /* do_not_lease */)
	assert.NoError(t, err)

	// Generic.Client.Info has two source preconditions in parallel
	assert.Equal(t, len(tasks), 2)

	// Cancelling the flow will notify the client immediately.
	response, err := flows.CancelFlow(
		context.Background(),
		self.ConfigObj, self.client_id, flow_id, "username")
	require.NoError(t, err)
	require.Equal(t, response.FlowId, flow_id)

	// Cancelling a flow simply schedules a cancel message for the
	// client and removes all pending tasks.
	tasks, err = clients.GetClientTasks(self.ConfigObj,
		self.client_id, true /* do_not_lease */)
	assert.NoError(t, err)
	assert.Equal(t, len(tasks), 1)

	// Client will cancel all in flight queries from this session
	// id.
	require.Equal(t, tasks[0].SessionId, flow_id)
	require.NotNil(t, tasks[0].Cancel)

	// The flow must be marked as cancelled with an error.
	collection_context := &flows_proto.ArtifactCollectorContext{}
	path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	err = db.GetSubject(self.ConfigObj, path_manager.Path(), collection_context)
	require.NoError(t, err)

	require.Regexp(t, regexp.MustCompile("Cancelled by username"),
		collection_context.Status)

	require.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		collection_context.State)
}

// Test an unknown flow. What happens when the server receives a
// message to an unknown flow.
func (self *ServerTestSuite) TestUnknownFlow() {
	t := self.T()

	db, err := datastore.GetDB(self.ConfigObj)
	require.NoError(t, err)

	runner := flows.NewFlowRunner(self.ConfigObj)
	defer runner.Close()

	// Send a message to a random non-existant flow from client.
	flow_id := "F.NONEXISTENT"
	runner.ProcessSingleMessage(
		context.Background(),
		&crypto_proto.VeloMessage{
			Source:      self.client_id,
			SessionId:   flow_id,
			VQLResponse: &actions_proto.VQLResponse{},
		})

	// This should send a cancellation message to the client.
	tasks, err := clients.GetClientTasks(self.ConfigObj,
		self.client_id, true /* do_not_lease */)
	assert.NoError(t, err)
	assert.Equal(t, len(tasks), 1)

	// Client will cancel all in flight queries from this session
	// id.
	require.Equal(t, tasks[0].SessionId, flow_id)
	require.NotNil(t, tasks[0].Cancel)

	// The flow does not exist - make sure it still does not.
	collection_context := &flows_proto.ArtifactCollectorContext{}
	path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	err = db.GetSubject(self.ConfigObj, path_manager.Path(), collection_context)
	require.Error(t, err, os.ErrNotExist)
}

// Test flow archiving
func (self *ServerTestSuite) TestFlowArchives() {
	t := self.T()

	db, err := datastore.GetDB(self.ConfigObj)
	require.NoError(t, err)

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	// Attempt to archive a running flow.
	_, err = flows.ArchiveFlow(
		self.ConfigObj, self.client_id, flow_id, "username")
	require.Error(t, err)

	// Cancelling the flow will notify the client immediately.

	// Now cancel the same flow.
	response, err := flows.CancelFlow(
		context.Background(),
		self.ConfigObj, self.client_id, flow_id, "username")
	require.NoError(t, err)
	require.Equal(t, response.FlowId, flow_id)

	// Now archive the flow - should work because the flow is terminated.
	res, err := flows.ArchiveFlow(
		self.ConfigObj, self.client_id, flow_id, "username")
	require.NoError(t, err)
	require.Equal(t, res.FlowId, flow_id)

	// The flow must be marked as archived.
	collection_context := &flows_proto.ArtifactCollectorContext{}
	path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	err = db.GetSubject(self.ConfigObj, path_manager.Path(), collection_context)
	require.NoError(t, err)

	require.Regexp(t, regexp.MustCompile("Archived by username"),
		collection_context.Status)

	require.Equal(self.T(), flows_proto.ArtifactCollectorContext_ARCHIVED,
		collection_context.State)
}

func TestServerTestSuite(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}
