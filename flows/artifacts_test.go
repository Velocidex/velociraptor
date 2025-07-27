package flows

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/uploads"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/filesystem"
	_ "www.velocidex.com/golang/velociraptor/vql/networking"
	_ "www.velocidex.com/golang/velociraptor/vql/windows"
	_ "www.velocidex.com/golang/velociraptor/vql/windows/filesystems"
)

var (
	nilTime         = time.Unix(0, 0)
	filename        = accessors.MustNewWindowsNTFSPath("foo")
	sparse_filename = accessors.MustNewWindowsNTFSPath("sparse")
)

type TestRangeReader struct {
	*bytes.Reader
	ranges []uploads.Range
}

func (self *TestRangeReader) Ranges() []uploads.Range {
	return self.ranges
}

type TestSuite struct {
	test_utils.TestSuite
	client_id string
	flow_id   string
}

func (self *TestSuite) SetupTest() {
	self.TestSuite.SetupTest()
	self.LoadArtifactsIntoConfig([]string{`
name: System.Upload.Completion
type: CLIENT_EVENT
`, `
name: Generic.Client.Profile
type: CLIENT
`})

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = client_info_manager.Set(self.Ctx, &services.ClientInfo{
		ClientInfo: &actions_proto.ClientInfo{
			ClientId: self.client_id,
		},
	})
}

func (self *TestSuite) TestGetFlow() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(
		self.ConfigObj)
	assert.NoError(self.T(), err)

	request1 := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Creator:   utils.GetSuperuserName(self.ConfigObj),
		Artifacts: []string{"Generic.Client.Info"},
	}

	request2 := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Creator:   utils.GetSuperuserName(self.ConfigObj),
		Artifacts: []string{"Generic.Client.Profile"},
	}

	// Schedule new flows.
	ctx := self.Ctx
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	flow_ids := []string{}

	// Create 40 flows with 2 types.
	for i := 0; i < 20; i++ {
		flow_id, err := launcher.ScheduleArtifactCollection(
			ctx, self.ConfigObj,
			acl_managers.NullACLManager{},
			repository, request1, nil)
		assert.NoError(self.T(), err)

		flow_ids = append(flow_ids, flow_id)

		flow_id, err = launcher.ScheduleArtifactCollection(
			ctx, self.ConfigObj,
			acl_managers.NullACLManager{},
			repository, request2, nil)
		assert.NoError(self.T(), err)

		flow_ids = append(flow_ids, flow_id)
	}

	// Get all the responses - ask for 100 results if available
	// but only 40 are there.
	api_response, err := launcher.GetFlows(self.Ctx, self.ConfigObj,
		self.client_id, result_sets.ResultSetOptions{}, 0, 100)
	assert.NoError(self.T(), err)

	// There should be 40 flows (2 sets of each)
	assert.Equal(self.T(), 40, len(api_response.Items))
}

func (self *TestSuite) TestRetransmission() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(
		self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Creator:   utils.GetSuperuserName(self.ConfigObj),
		Artifacts: []string{"Generic.Client.Info"},
	}

	// Schedule a new flow.
	ctx := self.Ctx
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, self.ConfigObj,
		acl_managers.NullACLManager{},
		repository, request, nil)
	assert.NoError(self.T(), err)

	// Send one row.
	message := &crypto_proto.VeloMessage{
		Source:     self.client_id,
		SessionId:  flow_id,
		RequestId:  1,
		ResponseId: uint64(time.Now().UnixNano()),
		VQLResponse: &actions_proto.VQLResponse{
			JSONLResponse: "{}",
			TotalRows:     1,
			Query: &actions_proto.VQLRequest{
				Name: "Generic.Client.Info/BasicInformation",
			},
		},
	}

	runner := NewLegacyFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close(self.Ctx)

	// Retransmit the same row again - this can happen if the
	// server is loaded and the client is re-uploading the same
	// payload multiple times.
	runner = NewLegacyFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close(self.Ctx)

	// Load the collection context and see what happened.
	collection_context, err := LoadCollectionContext(self.Ctx, self.ConfigObj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// The flow should have only a single row though.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(1))

}

func (self *TestSuite) TestResourceLimits() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)
	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Creator:   utils.GetSuperuserName(self.ConfigObj),
		Artifacts: []string{"Generic.Client.Info"},

		// Only accept 5 rows.
		MaxRows: 5,
	}

	// Schedule a new flow.
	ctx := self.Ctx
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx,
		self.ConfigObj,
		acl_managers.NullACLManager{},
		repository, request, nil)
	assert.NoError(self.T(), err)

	// Drain messages to the client.
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	var messages []*crypto_proto.VeloMessage

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		messages, err = client_info_manager.GetClientTasks(
			self.Ctx, self.client_id)
		assert.NoError(self.T(), err)
		return len(messages) == 2
	})

	// The Generic.Client.Info has two source conditions so it
	// contains two queries. To maintain backwards compatibility with
	// older clients, GetClientTasks should have two old style
	// VQLClientAction request with the first request incorporating
	// the FlowRequest message. Old clients will ignore the old
	// requests and new clients will ignore the old style requests.
	assert.Equal(self.T(), len(messages), 2)
	assert.True(self.T(), messages[0].FlowRequest != nil)
	assert.True(self.T(), messages[0].VQLClientAction != nil)
	assert.True(self.T(), messages[1].VQLClientAction != nil)
	assert.Equal(self.T(), len(messages[0].FlowRequest.VQLClientActions), 2)

	// Send one row.
	message := &crypto_proto.VeloMessage{
		Source:     self.client_id,
		SessionId:  flow_id,
		RequestId:  1,
		ResponseId: 2,
		VQLResponse: &actions_proto.VQLResponse{
			JSONLResponse: "{}",
			TotalRows:     1,
			Query: &actions_proto.VQLRequest{
				Name: "Generic.Client.Info/BasicInformation",
			},
		},
	}

	runner := NewLegacyFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close(self.Ctx)

	// Load the collection context and see what happened.
	collection_context, err := LoadCollectionContext(self.Ctx, self.ConfigObj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// Collection has 1 row and it is still in the running state.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(1))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_RUNNING)

	// Send another row
	message.ResponseId++
	runner = NewLegacyFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close(self.Ctx)

	// Load the collection context and see what happened.
	collection_context, err = LoadCollectionContext(self.Ctx, self.ConfigObj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// Collection has 2 rows and it is still in the running state.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(2))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_RUNNING)

	// Now send 5 rows in one message. We should accept the 5 rows
	// but terminate the flow due to resource exhaustion.
	message.VQLResponse.TotalRows = 5
	message.ResponseId++
	runner = NewLegacyFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close(self.Ctx)

	// Load the collection context and see what happened.
	collection_context, err = LoadCollectionContext(self.Ctx, self.ConfigObj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// Collection has 7 rows and it is still in the running state.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(7))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_ERROR)

	assert.Contains(self.T(), collection_context.Status, "Row count exceeded")

	// Make sure a cancel message was sent to the client.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		messages, err = client_info_manager.PeekClientTasks(
			self.Ctx, self.client_id)
		assert.NoError(self.T(), err)
		return len(messages) == 1
	})

	assert.Equal(self.T(), len(messages), 1)
	assert.NotNil(self.T(), messages[0].Cancel)

	// Another message arrives from the client - this happens
	// usually because the client has not received the cancel yet
	// and is already sending the next message in the queue.
	message.ResponseId++
	runner = NewLegacyFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close(self.Ctx)

	// We still collect these rows but the flow is still in the
	// error state. We do this so we dont lose the last few
	// messages which are still in flight.
	collection_context, err = LoadCollectionContext(self.Ctx, self.ConfigObj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(12))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_ERROR)
}

func (self *TestSuite) TestClientUploaderStoreFile() {
	resp := responder.TestResponderWithFlowId(
		self.ConfigObj, "TestClientUploaderStoreFile")
	uploader := uploads.NewVelociraptorUploader(self.Ctx, nil, 0, resp)
	defer uploader.Close()

	// Just a normal file with two regular ranges.
	reader := &TestRangeReader{
		Reader: bytes.NewReader([]byte(
			"Hello world hello world")),
		ranges: []uploads.Range{
			{Offset: 0, Length: 6, IsSparse: false},
			{Offset: 6, Length: 6, IsSparse: false},
		},
	}

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	uploader.Upload(self.Ctx, scope,
		filename, "ntfs", nil, 1000,
		nilTime, nilTime, nilTime, nilTime, 0, reader)

	// Get a new collection context.
	collection_context := NewCollectionContext(self.Ctx, self.ConfigObj)
	collection_context.ArtifactCollectorContext = flows_proto.ArtifactCollectorContext{
		SessionId:           self.flow_id,
		ClientId:            self.client_id,
		OutstandingRequests: 1,
		Request: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Generic.Client.Info"},
		},
	}

	for _, response := range resp.Drain.WaitForStatsMessage(self.T()) {
		response.Source = self.client_id
		err := ArtifactCollectorProcessOneMessage(self.Ctx,
			self.ConfigObj, collection_context, response)
		assert.NoError(self.T(), err)
	}

	// Close the context should force uploaded files to be
	// flushed.
	closeContext(self.Ctx, self.ConfigObj, collection_context)

	assert.Equal(self.T(), collection_context.TotalUploadedFiles, uint64(1))

	// Total bytes actually delivered and expected.
	assert.Equal(self.T(), collection_context.TotalUploadedBytes, uint64(12))
	assert.Equal(self.T(), collection_context.TotalExpectedUploadedBytes, uint64(12))

	// Debug the entire filestore
	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	// Check the file content is there
	flow_path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id)
	assert.Equal(self.T(),
		test_utils.FileReadAll(self.T(), self.ConfigObj,
			flow_path_manager.GetUploadsFile(
				"ntfs", "foo", []string{"foo"}).Path()),
		"Hello world ")

	// Check the upload metadata file.
	upload_metadata_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		flow_path_manager.UploadMetadata())

	assert.Equal(self.T(), len(upload_metadata_rows), 1)

	// The vfs_path indicates only the client's path
	vfs_path, _ := upload_metadata_rows[0].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path, "foo")

	// The _Components field is the path to the filestore components
	vfs_components, _ := upload_metadata_rows[0].GetStrings("_Components")
	assert.Equal(self.T(), vfs_components,
		flow_path_manager.GetUploadsFile("ntfs", "foo", []string{"foo"}).
			Path().Components())

	file_size, _ := upload_metadata_rows[0].GetInt64("file_size")
	assert.Equal(self.T(), file_size, int64(12))

	uploaded_size, _ := upload_metadata_rows[0].GetInt64("uploaded_size")
	assert.Equal(self.T(), uploaded_size, int64(12))

	// Check the System.Upload.Completion event.
	artifact_path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj, self.client_id, self.flow_id,
		"System.Upload.Completion")
	assert.NoError(self.T(), err)

	event_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		artifact_path_manager.Path())

	assert.Equal(self.T(), len(event_rows), 1)

	vfs_path, _ = event_rows[0].GetString("VFSPath")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "foo", []string{"foo"}).
			Path().AsClientPath())

	file_size, _ = event_rows[0].GetInt64("Size")
	assert.Equal(self.T(), file_size, int64(12))

	uploaded_size, _ = event_rows[0].GetInt64("UploadedSize")
	assert.Equal(self.T(), uploaded_size, int64(12))
}

// Schedule the flow and drain its messages to emulate it being
// inflight.
func (self *TestSuite) scheduleFlow() {
	closer := utils.SetFlowIdForTests(self.flow_id)
	defer closer()

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(
		self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Creator:   utils.GetSuperuserName(self.ConfigObj),
		Artifacts: []string{"Generic.Client.Info"},
	}

	flow_id, err := launcher.ScheduleArtifactCollection(
		self.Ctx, self.ConfigObj,
		acl_managers.NullACLManager{},
		repository, request, nil)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), flow_id, self.flow_id)

	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	_, err = client_info_manager.GetClientTasks(
		self.Ctx, self.client_id)
	assert.NoError(self.T(), err)
}

// Just a normal collection with error log - receive some rows and an
// ok status but an error log. NOTE: Earlier versions would maintain
// flow state on the server, but in recent versions flow state is
// maintained on the client. This means the client flow runner just
// writes exactly what FlowStats is sending.
func (self *TestSuite) TestCollectionCompletionErrorLogWithOkStatus() {

	self.scheduleFlow()

	// Emulate messages being sent from the client. Clients maintain
	// flow state so nothing happens until FlowStats is sent.
	flow := self.testCollectionCompletion(1, []*crypto_proto.VeloMessage{
		{
			SessionId: self.flow_id,
			Source:    self.client_id,
			LogMessage: &crypto_proto.LogMessage{
				NumberOfRows: 1,
				Jsonl:        `{"client_time":1,"level":"ERROR","message":"Error: This is an error"}\n`,
				ErrorMessage: "Error: This is an error",
			},
		},
		{
			SessionId: self.flow_id,
			Source:    self.client_id,
			VQLResponse: &actions_proto.VQLResponse{
				JSONLResponse: "{}",
				TotalRows:     1,
				Query: &actions_proto.VQLRequest{
					Name: "Generic.Client.Info/BasicInformation",
				},
			},
		},
		{
			SessionId: self.flow_id,
			Source:    self.client_id,
			FlowStats: &crypto_proto.FlowStats{
				// The final FlowStats is marked as final - the server
				// can use this to forward the event to any listeners.
				FlowComplete: true,

				// Stats are tracked per query.
				QueryStatus: []*crypto_proto.VeloStatus{{
					Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
					ErrorMessage: "Error: This is an error",
					Duration:     100,
					ResultRows:   1,
					NamesWithResponse: []string{
						"Generic.Client.Info/BasicInformation",
					},
				}},
			},
		},
	})

	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR, flow.State)
	assert.Contains(self.T(), flow.Status, "Error")
	assert.Equal(self.T(), uint64(1), flow.TotalCollectedRows)
	assert.Equal(self.T(), []string{"Generic.Client.Info/BasicInformation"},
		flow.ArtifactsWithResults)

	// Records the correct execution duration.
	assert.Equal(self.T(), int64(100), flow.ExecutionDuration)
}

// Just a normal collection - receive some rows and an ok status
func (self *TestSuite) TestCollectionCompletionMultiQueryOkStatus() {
	self.scheduleFlow()

	flow := self.testCollectionCompletion(1, []*crypto_proto.VeloMessage{
		{
			SessionId: self.flow_id,
			Source:    self.client_id,
			VQLResponse: &actions_proto.VQLResponse{
				JSONLResponse: "{}",
				TotalRows:     1,
				Query: &actions_proto.VQLRequest{
					Name: "Generic.Client.Info/BasicInformation",
				},
			},
		},
		{
			SessionId: self.flow_id,
			Source:    self.client_id,
			FlowStats: &crypto_proto.FlowStats{
				FlowComplete: true,
				// Two sources
				QueryStatus: []*crypto_proto.VeloStatus{
					{
						Status:     crypto_proto.VeloStatus_OK,
						Duration:   100,
						ResultRows: 1,
						NamesWithResponse: []string{
							"Generic.Client.Info/BasicInformation",
						},
					}, {
						Status:     crypto_proto.VeloStatus_OK,
						Duration:   125,
						ResultRows: 5,
						NamesWithResponse: []string{
							"Generic.Client.Info/Users",
						},
					}},
			},
		},
	})

	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED,
		flow.State)

	// The duration represents the longest duration of all queries.
	assert.Equal(self.T(), int64(125), flow.ExecutionDuration)
	assert.Equal(self.T(), uint64(6), flow.TotalCollectedRows)
	assert.Equal(self.T(), []string{
		"Generic.Client.Info/BasicInformation",
		"Generic.Client.Info/Users",
	}, flow.ArtifactsWithResults)
}

// Helper for replaying client messages through the client flow
// runner. This function blocks until the runner sends a
// System.Flow.Completion message then captures and returns the flow
// object sent on that event queue.
func (self *TestSuite) testCollectionCompletion(
	outstanding_requests int64,
	requests []*crypto_proto.VeloMessage) *flows_proto.ArtifactCollectorContext {
	// Get a new collection context.
	collection_context := NewCollectionContext(self.Ctx, self.ConfigObj)
	collection_context.ArtifactCollectorContext = flows_proto.ArtifactCollectorContext{
		SessionId:           self.flow_id,
		ClientId:            self.client_id,
		State:               flows_proto.ArtifactCollectorContext_RUNNING,
		OutstandingRequests: outstanding_requests,
		Request: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Generic.Client.Info"},
		},
	}

	runner := NewFlowRunner(self.Ctx, self.ConfigObj)

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}
	rows := []*ordereddict.Dict{}

	// Capture output from System.Flow.Completion
	err := journal.WatchQueueWithCB(self.Ctx, self.ConfigObj, wg,
		"System.Flow.Completion", "", func(ctx context.Context,
			config_obj *config_proto.Config,
			row *ordereddict.Dict) error {

			mu.Lock()
			defer mu.Unlock()

			rows = append(rows, row)
			return nil
		})
	assert.NoError(self.T(), err)

	for _, request := range requests {
		runner.ProcessSingleMessage(self.Ctx, request)
	}
	runner.Close(self.Ctx)

	vtesting.WaitUntil(10*time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		return len(rows) > 0
	})

	// Get the collection_context from the System.Flow.Completion
	// message.
	flow_any, pres := rows[0].Get("Flow")
	assert.True(self.T(), pres)

	flow, ok := flow_any.(*flows_proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	return flow
}

func (self *TestSuite) TestClientUploaderStoreSparseFile() {
	resp := responder.TestResponderWithFlowId(
		self.ConfigObj, "TestClientUploaderStoreSparseFile")
	uploader := uploads.NewVelociraptorUploader(self.Ctx, nil, 0, resp)
	defer uploader.Close()

	// A sparse file with one range of 6 bytes, a sparse 6 bytes
	// and another 6 byte data range.

	// This means file size is 18, transferred bytes 12.
	reader := &TestRangeReader{
		Reader: bytes.NewReader([]byte(
			"Hello world hello world")),
		ranges: []uploads.Range{
			{Offset: 0, Length: 6, IsSparse: false},
			{Offset: 6, Length: 6, IsSparse: true},
			{Offset: 12, Length: 6, IsSparse: false},
		},
	}

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	uploader.Upload(self.Ctx, scope,
		sparse_filename, "ntfs", nil, 1000,
		nilTime, nilTime, nilTime, nilTime, 0, reader)

	// Get a new collection context.
	collection_context := NewCollectionContext(self.Ctx, self.ConfigObj)
	collection_context.ArtifactCollectorContext = flows_proto.ArtifactCollectorContext{
		SessionId:           self.flow_id,
		ClientId:            self.client_id,
		OutstandingRequests: 1,
		Request:             &flows_proto.ArtifactCollectorArgs{},
	}

	for _, msg := range resp.Drain.WaitForStatsMessage(self.T()) {
		msg.Source = self.client_id
		if msg.FileBuffer != nil {
			// The uploader should be telling us the overall stats.
			assert.Equal(self.T(), msg.FileBuffer.Size, uint64(18))
			assert.Equal(self.T(), msg.FileBuffer.StoredSize, uint64(12))
		}

		ArtifactCollectorProcessOneMessage(self.Ctx, self.ConfigObj,
			collection_context, msg)
	}

	// Close the context should force uploaded files to be
	// flushed.
	closeContext(self.Ctx, self.ConfigObj, collection_context)

	// One file is uploaded
	assert.Equal(self.T(), collection_context.TotalUploadedFiles, uint64(1))

	// Total bytes actually delivered and expected.
	assert.Equal(self.T(), collection_context.TotalUploadedBytes, uint64(12))
	assert.Equal(self.T(), collection_context.TotalExpectedUploadedBytes, uint64(12))

	// Debug the entire filestore
	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	// Check the file content is there
	flow_path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id)
	assert.Equal(self.T(),
		test_utils.FileReadAll(self.T(), self.ConfigObj,
			flow_path_manager.GetUploadsFile(
				"ntfs", "sparse", []string{"sparse"}).Path()),
		"Hello hello ")

	// Check the upload metadata file.
	upload_metadata_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		flow_path_manager.UploadMetadata())

	// There should be two rows - one for the file and one for the index.
	assert.Equal(self.T(), len(upload_metadata_rows), 2)

	vfs_path, _ := upload_metadata_rows[0].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path, "sparse")

	vfs_components, _ := upload_metadata_rows[0].GetStrings("_Components")
	assert.Equal(self.T(), vfs_components,
		flow_path_manager.GetUploadsFile(
			"ntfs", "sparse", []string{"sparse"}).Path().Components())

	// The file is actually 18 bytes on the client.
	file_size, _ := upload_metadata_rows[0].GetInt64("file_size")
	assert.Equal(self.T(), file_size, int64(18))

	// But we have 12 bytes in total uploaded
	uploaded_size, _ := upload_metadata_rows[0].GetInt64("uploaded_size")
	assert.Equal(self.T(), uploaded_size, int64(12))

	// Second row is for the index.
	vfs_path, _ = upload_metadata_rows[1].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path, "sparse.idx")

	vfs_components, _ = upload_metadata_rows[0].GetStrings("_Components")
	assert.Equal(self.T(), vfs_components,
		flow_path_manager.GetUploadsFile(
			"ntfs", "sparse", []string{"sparse"}).IndexPath().Components())

	// Check the System.Upload.Completion event.
	artifact_path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj, self.client_id, self.flow_id,
		"System.Upload.Completion")
	assert.NoError(self.T(), err)

	event_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		artifact_path_manager.Path())

	assert.Equal(self.T(), len(event_rows), 1)

	vfs_path, _ = event_rows[0].GetString("VFSPath")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile(
			"ntfs", "sparse", []string{"sparse"}).Path().AsClientPath())

	file_size, _ = event_rows[0].GetInt64("Size")
	assert.Equal(self.T(), file_size, int64(18))

	uploaded_size, _ = event_rows[0].GetInt64("UploadedSize")
	assert.Equal(self.T(), uploaded_size, int64(12))
}

// This test only runs on windows.
func (self *TestSuite) TestClientUploaderStoreSparseFileNTFS() {
	if runtime.GOOS != "windows" {
		self.T().Skip()
		return
	}

	filename := "C:\\Users\\sparse.txt"

	cmd := exec.Command("FSUtil", "File", "CreateNew", filename, "0x100000")
	cmd.CombinedOutput()

	cmd = exec.Command("FSUtil", "Sparse", "SetFlag", filename)
	cmd.CombinedOutput()

	cmd = exec.Command("FSUtil", "Sparse", "SetRange", filename, "0", "0x100000")
	cmd.CombinedOutput()

	scope := vql_subsystem.MakeScope().AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}))
	accessor, err := accessors.GetAccessor("ntfs", scope)
	assert.NoError(self.T(), err)

	fd, err := accessor.Open(filename)
	assert.NoError(self.T(), err)

	resp := responder.TestResponderWithFlowId(
		self.ConfigObj, "TestClientUploaderStoreSparseFileNTFS")
	uploader := uploads.NewVelociraptorUploader(self.Ctx, nil, 0, resp)
	defer uploader.Close()

	// Upload the file to the responder.
	uploader.Upload(self.Ctx, scope,
		sparse_filename, "ntfs", nil, 1000,
		nilTime, nilTime, nilTime, nilTime, 0, fd)

	// Get a new collection context.
	collection_context := NewCollectionContext(self.Ctx, self.ConfigObj)
	collection_context.ArtifactCollectorContext = flows_proto.ArtifactCollectorContext{
		SessionId: self.flow_id,
		ClientId:  self.client_id,
		Request:   &flows_proto.ArtifactCollectorArgs{},
	}

	// Process it.
	for _, resp := range resp.Drain.WaitForStatsMessage(self.T()) {
		resp.Source = self.client_id
		ArtifactCollectorProcessOneMessage(self.Ctx, self.ConfigObj,
			collection_context, resp)
	}

	// Close the context should force uploaded files to be
	// flushed.
	closeContext(self.Ctx, self.ConfigObj, collection_context)

	// One file is uploaded
	assert.Equal(self.T(), collection_context.TotalUploadedFiles, uint64(1))

	// Total bytes actually delivered and expected is 0 because the file is sparse.
	assert.Equal(self.T(), collection_context.TotalUploadedBytes, uint64(0))
	assert.Equal(self.T(), collection_context.TotalExpectedUploadedBytes, uint64(0))

	// Debug the entire filestore
	test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	// Check the file content is there
	flow_path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id)
	assert.Equal(self.T(),
		test_utils.FileReadAll(self.T(), self.ConfigObj,
			flow_path_manager.GetUploadsFile(
				"ntfs", "sparse", []string{"sparse"}).Path()),
		"")

	// Check the upload metadata file.
	upload_metadata_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		flow_path_manager.UploadMetadata())

	// There should be two rows - one for the file and one for the index.
	assert.Equal(self.T(), len(upload_metadata_rows), 2)

	vfs_path, _ := upload_metadata_rows[0].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path, "sparse")

	vfs_components, _ := upload_metadata_rows[0].GetStrings("_Components")
	assert.Equal(self.T(), vfs_components,
		flow_path_manager.GetUploadsFile(
			"ntfs", "sparse", []string{"sparse"}).Path().Components())

	// The file is actually 0x100000 bytes on the client.
	file_size, _ := upload_metadata_rows[0].GetInt64("file_size")
	assert.Equal(self.T(), file_size, int64(0x100000))

	// But we have 0 bytes in total uploaded because the entire file is sparse
	uploaded_size, _ := upload_metadata_rows[0].GetInt64("uploaded_size")
	assert.Equal(self.T(), uploaded_size, int64(0))

	// Second row is for the index.
	vfs_path, _ = upload_metadata_rows[1].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path, "sparse.idx")

	vfs_components, _ = upload_metadata_rows[0].GetStrings("_Components")
	assert.Equal(self.T(), vfs_components,
		flow_path_manager.GetUploadsFile(
			"ntfs", "sparse", []string{"sparse"}).IndexPath().Components())

	// Check the System.Upload.Completion event.
	artifact_path_manager, err := artifacts.NewArtifactPathManager(
		self.Ctx, self.ConfigObj, self.client_id, self.flow_id,
		"System.Upload.Completion")
	assert.NoError(self.T(), err)

	event_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		artifact_path_manager.Path())

	assert.Equal(self.T(), len(event_rows), 1)

	vfs_path, _ = event_rows[0].GetString("VFSPath")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile(
			"ntfs", "sparse", []string{"sparse"}).Path().AsClientPath())

	file_size, _ = event_rows[0].GetInt64("Size")
	assert.Equal(self.T(), file_size, int64(0x100000))

	uploaded_size, _ = event_rows[0].GetInt64("UploadedSize")
	assert.Equal(self.T(), uploaded_size, int64(0))
}

func TestArtifactCollection(t *testing.T) {
	suite.Run(t, &TestSuite{
		client_id: "C.12312",
		flow_id:   "F.1232",
	})
}

func getFlowIds(in []*flows_proto.ArtifactCollectorContext) []string {
	res := []string{}
	for _, i := range in {
		res = append(res, i.SessionId)
	}
	return res
}
