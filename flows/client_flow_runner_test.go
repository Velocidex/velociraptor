package flows_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
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
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_client "www.velocidex.com/golang/velociraptor/crypto/client"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	file_store_api "www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

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
name: Server.Internal.ClientPing
type: SERVER
`, `
name: Server.Internal.ClientInfoSnapshot
type: SERVER
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
`, `
name: Server.Internal.Alerts
type: SERVER_EVENT
`, `
name: Server.Internal.ClientScheduled
type: SERVER_EVENT
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
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.HuntDispatcher = true
	self.ConfigObj.Services.ClientMonitoring = true
	self.ConfigObj.Services.Interrogation = true
	self.ConfigObj.Client.Concurrency = 1

	self.LoadArtifactsIntoConfig(mock_definitions)

	var err error
	self.TestSuite.SetupTest()

	self.server, err = server.NewServer(self.Sm.Ctx, self.ConfigObj, self.Sm.Wg)
	require.NoError(self.T(), err)

	self.client_crypto, err = crypto_client.NewClientCryptoManager(self.Ctx,
		self.ConfigObj, []byte(self.ConfigObj.Writeback.PrivateKey))
	require.NoError(self.T(), err)

	_, err = self.client_crypto.AddCertificate(self.ConfigObj, []byte(
		self.ConfigObj.Frontend.Certificate))

	require.NoError(self.T(), err)

	self.client_id = self.client_crypto.ClientId()

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{ClientId: self.client_id},
	})
	assert.NoError(self.T(), err)
}

// Check that flows go through their respective states
func (self *ServerTestSuite) TestFlowStates() {
	var flow_ids []string

	time_origin := int64(100)

	closer := utils.MockTime(utils.NewMockClock(time.Unix(time_origin, 0)))
	defer closer()

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Create 5 flows to the client.
	for i := 0; i < 5; i++ {
		flow_id, err := self.createArtifactCollection()
		require.NoError(self.T(), err)

		flow_ids = append(flow_ids, flow_id)

		flow_details, err := launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.client_id, flow_id)
		assert.NoError(self.T(), err)

		// Our initial state is RUNNING
		assert.Equal(self.T(), flow_details.Context.State,
			flows_proto.ArtifactCollectorContext_RUNNING)
	}

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Should be one flow request scheduled.
	messages, err := client_info_manager.PeekClientTasks(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	// We should have 5 messages waiting for the client - one for each flow.
	assert.Equal(self.T(), len(messages), 5)

	// Now draine some messages to the client - concurrency is set to
	// 1 so we will pull 3 messages from the queue.
	tasks, err := client_info_manager.GetClientTasks(self.Ctx, self.client_id)

	// For backwards compatibility tasks are expanded so we only care
	// about the flow requests in current version.
	requests := getFlowRequests(tasks)
	assert.Equal(self.T(), len(requests), 3)

	// Two tasks remain in the queue.
	tasks_remaining, err := client_info_manager.PeekClientTasks(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks_remaining), 2)

	// We do not send any more tasks to the client until these ones
	// are done
	tasks, err = client_info_manager.GetClientTasks(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(tasks), 0)

	// The client info record shows which flows are currently in
	// flight.
	var client_info *services.ClientInfo
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		client_info, err = client_info_manager.Get(self.Ctx, self.client_id)
		assert.NoError(self.T(), err)

		return len(client_info.InFlightFlows) > 0
	})

	// Each flow is set to be in flight now
	assert.Equal(self.T(), len(client_info.InFlightFlows), 3)

	for in_flight := range client_info.InFlightFlows {
		flow_details, err := launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.client_id, in_flight)
		assert.NoError(self.T(), err)

		// The flow is in the WAITING state now until the client sends
		// any status updates
		assert.Equal(self.T(), flow_details.Context.State,
			flows_proto.ArtifactCollectorContext_WAITING)

		// Emulate a response from the flow - just some progress
		// report - this acks that we are running the query now.
		runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
		runner.ProcessSingleMessage(self.Ctx,
			&crypto_proto.VeloMessage{
				Source:    self.client_id,
				SessionId: in_flight,
				RequestId: constants.ProcessVQLResponses,
				FlowStats: &crypto_proto.FlowStats{
					QueryStatus: []*crypto_proto.VeloStatus{
						{
							Status: crypto_proto.VeloStatus_PROGRESS,
						},
					},
				},
			})
		runner.Close(self.Ctx)

		flow_details, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.client_id, in_flight)
		assert.NoError(self.T(), err)

		// The flow is in the IN_PROGRESS state now.
		assert.Equal(self.T(), flow_details.Context.State,
			flows_proto.ArtifactCollectorContext_IN_PROGRESS)

		// Advance time by 10 minutes to emulate client going away for
		// a while.
		time_origin += 600
		clock := utils.NewMockClock(time.Unix(time_origin, 0))
		closer := utils.MockTime(clock)
		defer closer()

		// The flow is in the UNRESPONSIVE state now.
		vtesting.WaitUntil(1*time.Second, self.T(), func() bool {
			flow_details, err = launcher.GetFlowDetails(
				self.Ctx, self.ConfigObj, services.GetFlowOptions{},
				self.client_id, in_flight)
			assert.NoError(self.T(), err)

			// Move time forward on each iteration
			time_origin += 60
			clock.Set(time.Unix(time_origin, 0))

			return flow_details.Context.State == flows_proto.ArtifactCollectorContext_UNRESPONSIVE
		})
		assert.Equal(self.T(), flow_details.Context.State,
			flows_proto.ArtifactCollectorContext_UNRESPONSIVE)

		// Lets complete the flow.
		runner = flows.NewFlowRunner(self.Ctx, self.ConfigObj)
		runner.ProcessSingleMessage(self.Ctx,
			&crypto_proto.VeloMessage{
				Source:    self.client_id,
				SessionId: in_flight,
				RequestId: constants.ProcessVQLResponses,
				FlowStats: &crypto_proto.FlowStats{
					QueryStatus: []*crypto_proto.VeloStatus{
						{
							Status: crypto_proto.VeloStatus_OK,
						},
					},
					FlowComplete: true,
				},
			})
		runner.Close(self.Ctx)

		flow_details, err = launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.client_id, in_flight)
		assert.NoError(self.T(), err)

		// The flow is in the FINISHED state now.
		assert.Equal(self.T(), flow_details.Context.State,
			flows_proto.ArtifactCollectorContext_FINISHED)
	}

	// The client record should not have any in flight flows now.
	vtesting.WaitUntil(15*time.Second, self.T(), func() bool {
		client_info, err = client_info_manager.Get(self.Ctx, self.client_id)
		assert.NoError(self.T(), err)

		return len(client_info.InFlightFlows) == 0
	})

	// Lets get the next batch of flows off the queue. This is now
	// possible because the in_flight flows have been cleared.
	tasks, err = client_info_manager.GetClientTasks(self.Ctx, self.client_id)

	// For backwards compatibility tasks are expanded so we only care
	// about the flow requests in current version.
	requests = getFlowRequests(tasks)
	assert.Equal(self.T(), len(requests), 2)

	// Now lets simulate the client crashing... Advance time by 10 minutes
	time_origin += 600

	clock := utils.NewMockClock(time.Unix(time_origin, 0))
	closer = utils.MockTime(clock)
	defer closer()

	vtesting.WaitUntil(15*time.Second, self.T(), func() bool {
		// Get the next set of tasks for the client.
		tasks, err = client_info_manager.GetClientTasks(self.Ctx, self.client_id)
		assert.NoError(self.T(), err)

		time_origin += 60
		clock.Set(time.Unix(time_origin, 0))

		return len(tasks) == 1
	})

	// The server will send the client a flow stats request - please
	// tell me about these flows. If the client really crashed it will
	// know nothing about these flows and send an automated
	// termination for them.
	assert.Equal(self.T(), len(tasks), 1)
	assert.Equal(self.T(), len(tasks[0].FlowStatsRequest.FlowId), 2)
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

	self.server.ProcessSingleUnauthenticatedMessage(self.Ctx,
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

	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	defer runner.Close(self.Ctx)

	// Set a new event monitoring table
	client_event_manager, err := services.ClientEventManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = client_event_manager.SetClientMonitoringState(self.Ctx,
		self.ConfigObj, "",
		&flows_proto.ClientEventTable{
			Artifacts: &flows_proto.ArtifactCollectorArgs{
				Artifacts: []string{"Generic.Client.Stats"},
			},
		})
	require.NoError(t, err)

	// The version of the currently installed table.
	version := client_event_manager.GetClientMonitoringState().Version

	// Wait a bit.
	time.Sleep(time.Second)

	// Send a message from client to trigger check
	runner.ProcessMessages(self.Ctx, &crypto.MessageInfo{
		Source: self.client_id,
	})

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	var tasks []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		tasks, err = client_info_manager.PeekClientTasks(self.Ctx, self.client_id)
		assert.NoError(t, err)
		return len(tasks) == 1
	})

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
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	defer runner.Close(self.Ctx)

	db, err := datastore.GetDB(self.ConfigObj)
	require.NoError(self.T(), err)

	// The hunt will launch the Generic.Client.Info on the client.
	expected := api.MakeCollectorRequest(
		self.client_id, "Generic.Client.Info")
	expected.MaxUploadBytes = 1073741824

	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(self.T(), err)

	new_hunt, err := hunt_dispatcher.CreateHunt(
		self.Ctx, self.ConfigObj,
		acl_managers.NullACLManager{},
		&api_proto.Hunt{
			State:        api_proto.Hunt_RUNNING,
			StartRequest: expected,
		})
	assert.NoError(t, err)

	// Check for hunt object in the data store.
	hunt := &api_proto.Hunt{}
	hunt_path_manager := paths.NewHuntPathManager(new_hunt.HuntId)
	err = db.GetSubject(self.ConfigObj, hunt_path_manager.Path(), hunt)
	require.NoError(t, err)

	assert.NotNil(t, hunt.StartRequest.CompiledCollectorArgs)

	hunt.StartRequest.FlowId = ""
	hunt.StartRequest.CompiledCollectorArgs = nil
	expected.CompiledCollectorArgs = nil

	assert.Equal(t, hunt.StartRequest, expected)

	// Send a message from client to trigger check
	runner.ProcessMessages(self.Ctx, &crypto.MessageInfo{
		Source: self.client_id,
	})

	// Server should schedule the new hunt on the client.
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(t, err)

	var tasks []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		tasks, err = client_info_manager.PeekClientTasks(self.Ctx, self.client_id)
		assert.NoError(t, err)
		return len(tasks) == 1
	})

	// Task should be UpdateEventTable message.
	assert.Equal(t, tasks[0].SessionId, "F.Monitoring")
	require.NotNil(t, tasks[0].UpdateEventTable)

	// The client_info_manager will remember the last hunt timestamp
	stats, err := client_info_manager.GetStats(self.Ctx, self.client_id)
	assert.NoError(t, err)

	assert.Equal(t, stats.LastHuntTimestamp, hunt.StartTime)
}

func (self *ServerTestSuite) RequiredFilestoreContains(
	filename file_store_api.FSPathSpec, regex string) {

	file_store_factory := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	value, pres := file_store_factory.Get(datastore.AsFilestoreFilename(
		db, self.ConfigObj, filename))
	if !pres {
		self.T().FailNow()
	}

	require.Regexp(self.T(), regexp.MustCompile(regex), string(value))
}

// Receiving a response from the server to the monitoring flow will
// write the rows into a jsonl file in the client's monitoring area.
func (self *ServerTestSuite) TestMonitoring() {
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: constants.MONITORING_WELL_KNOWN_FLOW,
			VQLResponse: &actions_proto.VQLResponse{
				Columns: []string{
					"ClientId", "Timestamp", "Fqdn", "HuntId"},
				JSONLResponse: fmt.Sprintf(
					"{\"ClientId\": \"%s\", \"HuntId\": \"H.123\"}\n", self.client_id),
				TotalRows: 1,
				Query: &actions_proto.VQLRequest{
					Name: "Generic.Client.Stats",
				},
			},
		})
	runner.Close(self.Ctx)

	path_manager, err := artifacts.NewArtifactPathManager(self.Ctx, self.ConfigObj,
		self.client_id, constants.MONITORING_WELL_KNOWN_FLOW,
		"Generic.Client.Stats")
	assert.NoError(self.T(), err)

	self.RequiredFilestoreContains(path_manager.Path(), self.client_id)
}

// Receiving a response from the server to the monitoring flow will
// write the rows into a jsonl file in the client's monitoring area.
func (self *ServerTestSuite) TestMonitoringAlerts() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(1602103388, 0)))
	defer closer()

	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: constants.MONITORING_WELL_KNOWN_FLOW,
			LogMessage: &crypto_proto.LogMessage{
				Id:           1,
				NumberOfRows: 1,
				Jsonl:        `{"client_time": 10, "level":"ALERT","message": "{\"field1\": \"test\"}"}`,
				Artifact:     "Generic.Client.Stats",
				Level:        logging.ALERT,
			},
		})
	runner.Close(self.Ctx)

	golden := ordereddict.NewDict()
	fs := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	for _, path := range []string{
		"/server_artifacts/Server.Internal.Alerts/2020-10-07.json",
		"/server_artifacts/EventTest.Alert/2020-10-07.json",
	} {
		value, pres := fs.Get(path)
		if pres {
			golden.Set(path, strings.Split(string(value), "\n"))
		}
	}
	goldie.AssertJson(self.T(), "TestMonitoringAlerts", golden)
}

// Monitoring queries which upload data.
func (self *ServerTestSuite) TestMonitoringWithUpload() {
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: constants.MONITORING_WELL_KNOWN_FLOW,
			RequestId: constants.TransferWellKnownFlowId,
			FileBuffer: &actions_proto.FileBuffer{
				Pathspec: &actions_proto.PathSpec{
					Path:       "/etc/passwd",
					Components: []string{"etc", "passwd"},
				},
				Data: []byte("Hello"),
				Size: 10000,
			},
		})
	runner.Close(self.Ctx)

	path_manager := paths.NewFlowPathManager(
		self.client_id, "F.Monitoring").GetUploadsFile(
		"file", "/etc/passwd", []string{"etc", "passwd"})
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
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			LogMessage: &crypto_proto.LogMessage{
				Jsonl: "{\"message\":\"Foobar\"}\n",
			},
		})
	runner.Close(self.Ctx)

	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			LogMessage: &crypto_proto.LogMessage{
				Jsonl: "{\"message\":\"ZooBar\"}\n",
			},
		})
	runner.Close(self.Ctx)

	path_spec := paths.NewFlowPathManager(self.client_id, flow_id).Log()
	self.RequiredFilestoreContains(path_spec, "Foobar")
	self.RequiredFilestoreContains(path_spec, "ZooBar")
}

func (self *ServerTestSuite) TestScheduleCollection() {
	t := self.T()
	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Creator:   utils.GetSuperuserName(self.ConfigObj),
		Artifacts: []string{"Generic.Client.Info"},
	}

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(t, err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		request, nil)

	db, err := datastore.GetDB(self.ConfigObj)
	require.NoError(t, err)

	// Launching the artifact will schedule one query on the client.
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	var tasks []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		tasks, err = client_info_manager.PeekClientTasks(self.Ctx, self.client_id)
		assert.NoError(t, err)
		return len(tasks) == 1
	})

	// The request sends a single FlowRequest task with two queries
	assert.Equal(t, len(tasks[0].FlowRequest.VQLClientActions), 2)

	collection_context := &flows_proto.ArtifactCollectorContext{}
	path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	err = db.GetSubject(self.ConfigObj, path_manager.Path(), collection_context)
	require.NoError(t, err)

	assert.Equal(t, collection_context.Request, request)
}

// Schedule a flow in the database and return its flow id
func (self *ServerTestSuite) createArtifactCollection() (string, error) {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	require.NoError(self.T(), err)

	// Schedule a flow in the database.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository,
		&flows_proto.ArtifactCollectorArgs{
			ClientId:  self.client_id,
			Creator:   utils.GetSuperuserName(self.ConfigObj),
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
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	runner.ProcessSingleMessage(self.Ctx,
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
				Eof:    true,
			},
		})
	runner.Close(self.Ctx)

	flow_path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	self.RequiredFilestoreContains(
		flow_path_manager.GetUploadsFile(
			"file", "/tmp/foobar", []string{"tmp", "foobar"}).Path(),
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
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.ProcessVQLResponses,
			VQLResponse: &actions_proto.VQLResponse{
				Columns: []string{"ClientId", "Column1"},
				JSONLResponse: fmt.Sprintf(
					"{\"ClientId\": \"%s\", \"Column1\": \"Foo\"}\n", self.client_id),
				Query: &actions_proto.VQLRequest{
					Name: "Generic.Client.Info",
				},
			},
		})
	runner.Close(self.Ctx)

	flow_path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj,
		self.client_id, flow_id, "Generic.Client.Info")
	assert.NoError(self.T(), err)

	self.RequiredFilestoreContains(flow_path_manager.Path(), self.client_id)
}

// When VQLResponse messages are retransmitted we need to detect and
// remove them.
func (self *ServerTestSuite) TestVQLResponseRetransmission() {
	t := self.T()

	closer := utils.SetFlowIdForTests("F.1234")
	defer closer()

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	// Emulate a response from this flow.
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)

	// Retransmit the first message 10 times
	for i := 0; i < 10; i++ {
		runner.ProcessSingleMessage(self.Ctx,
			&crypto_proto.VeloMessage{
				Source:    self.client_id,
				SessionId: flow_id,
				RequestId: constants.ProcessVQLResponses,
				VQLResponse: &actions_proto.VQLResponse{
					Columns:       []string{"Row"},
					JSONLResponse: "{\"Row\": 1}\n",
					Query: &actions_proto.VQLRequest{
						Name: "Generic.Client.Info",
					},
					// The first row in this result set.
					TotalRows:     1,
					QueryStartRow: 0,
					Part:          0,
				},
			})
	}

	// Retransmit the second message 10 times
	for i := 0; i < 10; i++ {
		runner.ProcessSingleMessage(self.Ctx,
			&crypto_proto.VeloMessage{
				Source:    self.client_id,
				SessionId: flow_id,
				RequestId: constants.ProcessVQLResponses,
				VQLResponse: &actions_proto.VQLResponse{
					Columns:       []string{"Row"},
					JSONLResponse: "{\"Row\": 2}\n",
					Query: &actions_proto.VQLRequest{
						Name: "Generic.Client.Info",
					},
					// The first row in this result set.
					TotalRows:     1,
					QueryStartRow: 1,
					Part:          1,
				},
			})
	}

	runner.Close(self.Ctx)

	file_store_factory := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	value, _ := file_store_factory.Get(
		"/clients/" + self.client_id + "/artifacts/Generic.Client.Info/F.1234.json")

	goldie.Assert(self.T(), "TestVQLResponseRetransmission", value)
}

// Errors from the client kill the flow.
func (self *ServerTestSuite) TestErrorMessage() {
	t := self.T()

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	// Emulate a response from this flow.
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.ProcessVQLResponses,
			FlowStats: &crypto_proto.FlowStats{
				QueryStatus: []*crypto_proto.VeloStatus{
					{
						Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
						ErrorMessage: "Error generated.",
						Backtrace:    "I am a backtrace",
					},
				},
			},
		})
	runner.Close(self.Ctx)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	details, err := launcher.GetFlowDetails(
		self.Ctx, self.ConfigObj, services.GetFlowOptions{},
		self.client_id, flow_id)
	require.NoError(t, err)

	require.Regexp(self.T(), regexp.MustCompile("Error generated"),
		details.Context.Status)

	require.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR,
		details.Context.State)
}

// Successful status should terminate the flow.
func (self *ServerTestSuite) TestCompletions() {
	t := self.T()

	// Schedule a flow in the database.
	flow_id, err := self.createArtifactCollection()
	require.NoError(t, err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Emulate a response from this flow.
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)

	// Generic.Client.Info sends two requests, send status for one
	// message is complete but the other is still running.
	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.ProcessVQLResponses,
			FlowStats: &crypto_proto.FlowStats{
				QueryStatus: []*crypto_proto.VeloStatus{
					{Status: crypto_proto.VeloStatus_OK, QueryId: 1},
					{Status: crypto_proto.VeloStatus_PROGRESS, QueryId: 2},
				},
			},
		})
	defer runner.Close(self.Ctx)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		details, err := launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.client_id, flow_id)
		require.NoError(t, err)

		// Flow not complete yet - still an outstanding request.
		return flows_proto.ArtifactCollectorContext_IN_PROGRESS ==
			details.Context.State
	})

	// Now complete both queries
	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.ProcessVQLResponses,
			FlowStats: &crypto_proto.FlowStats{
				QueryStatus: []*crypto_proto.VeloStatus{
					{Status: crypto_proto.VeloStatus_OK, QueryId: 1},
					{Status: crypto_proto.VeloStatus_OK, QueryId: 2},
				},
			},
		})
	defer runner.Close(self.Ctx)

	vtesting.WaitUntil(5*time.Second, self.T(), func() bool {
		// Flow should be complete now that second response arrived.
		details, err := launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			self.client_id, flow_id)
		require.NoError(t, err)

		return flows_proto.ArtifactCollectorContext_FINISHED ==
			details.Context.State
	})
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
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		tasks, err := client_info_manager.PeekClientTasks(self.Ctx, self.client_id)
		assert.NoError(t, err)

		// Generic.Client.Info has two source preconditions in parallel
		return len(tasks) == 1 &&
			len(tasks[0].FlowRequest.VQLClientActions) == 2
	})

	// Cancelling the flow will notify the client immediately.
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(t, err)

	response, err := launcher.CancelFlow(self.Ctx,
		self.ConfigObj, self.client_id, flow_id, "username")
	require.NoError(t, err)
	require.Equal(t, response.FlowId, flow_id)

	// Cancelling a flow simply schedules a cancel message for the
	// client and removes all pending tasks.
	var tasks []*crypto_proto.VeloMessage
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		tasks, err = client_info_manager.PeekClientTasks(self.Ctx, self.client_id)
		assert.NoError(t, err)
		return len(tasks) == 1
	})

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

	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	defer runner.Close(self.Ctx)

	// Send a message to a random non-existant flow from client.
	flow_id := "F.NONEXISTENT"
	runner.ProcessSingleMessage(
		self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			FlowStats: &crypto_proto.FlowStats{
				QueryStatus: []*crypto_proto.VeloStatus{
					{Status: crypto_proto.VeloStatus_OK, QueryId: 1},
				},
			},
		})

	// We used to send cancellation message to the client, but this
	// too expensive for the server to keep track of. Now we just
	// write data in the flow as if it exists anyway.

	// The flow does not exist - make sure it still does not.
	collection_context := &flows_proto.ArtifactCollectorContext{}
	path_manager := paths.NewFlowPathManager(self.client_id, flow_id)
	err = db.GetSubject(self.ConfigObj, path_manager.Path(), collection_context)
	require.Error(t, err, os.ErrNotExist)

	// The flow stats are written as normal.
	err = db.GetSubject(self.ConfigObj, path_manager.Stats(), collection_context)
	assert.NoError(t, err)
}

// Test an unknown flow. What happens when the server receives a
// message to an unknown flow.
func (self *ServerTestSuite) TestMultipleFlowComplete() {
	time_origin := int64(100)

	closer := utils.MockTime(utils.NewMockClock(time.Unix(time_origin, 0)))
	defer closer()

	completions := ordereddict.NewDict()

	err := journal.WatchQueueWithCB(self.Ctx, self.ConfigObj, self.Wg,
		"System.Flow.Completion", "",
		func(ctx context.Context, config_obj *config_proto.Config,
			row *ordereddict.Dict) error {
			key := fmt.Sprintf("%d", completions.Len())
			completions.Set(key, row)
			return nil
		})
	assert.NoError(self.T(), err)

	flow_id, err := self.createArtifactCollection()
	require.NoError(self.T(), err)

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Now draine some messages to the client - this will trigger the
	// flow to be in flight.
	tasks, err := client_info_manager.GetClientTasks(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)
	assert.True(self.T(), len(tasks) > 0)

	// Check the client record
	client_record, err := client_info_manager.Get(self.Ctx, self.client_id)
	assert.NoError(self.T(), err)

	// Make sure the flow is now in flight.
	_, pres := client_record.InFlightFlows[flow_id]
	assert.True(self.T(), pres)

	// Emulate a response from the flow - This flow is completed.
	runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
	runner.ProcessSingleMessage(self.Ctx,
		&crypto_proto.VeloMessage{
			Source:    self.client_id,
			SessionId: flow_id,
			RequestId: constants.ProcessVQLResponses,
			FlowStats: &crypto_proto.FlowStats{
				QueryStatus: []*crypto_proto.VeloStatus{
					{
						Status: crypto_proto.VeloStatus_OK,
					},
				},
				FlowComplete: true,
			},
		})
	runner.Close(self.Ctx)

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		return completions.Len() > 0
	})

	// Now send more responses to this flow. For example, if the
	// client crashed. This should not send a second Completions
	// event!
	for i := 0; i < 10; i++ {
		runner := flows.NewFlowRunner(self.Ctx, self.ConfigObj)
		runner.ProcessSingleMessage(self.Ctx,
			&crypto_proto.VeloMessage{
				Source:    self.client_id,
				SessionId: flow_id,
				RequestId: constants.STATS_SINK,
				FlowStats: &crypto_proto.FlowStats{
					FlowComplete: true,
					QueryStatus: []*crypto_proto.VeloStatus{
						{
							Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
							ErrorMessage: "Flow not known - maybe the client crashed?",
						},
					},
				},
			})
		runner.Close(self.Ctx)
	}
	time.Sleep(time.Second / 2)

	// No more completion events are sent.
	if completions.Len() != 1 {
		json.Dump(completions)
	}
	assert.Equal(self.T(), completions.Len(), 1)
}

func getFlowRequests(messages []*crypto_proto.VeloMessage) (res []*crypto_proto.VeloMessage) {
	for _, m := range messages {
		if m.FlowRequest != nil {
			res = append(res, m)
		}
	}
	return res
}

func TestServerTestSuite(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}
