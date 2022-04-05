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
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
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
	self.LoadArtifacts([]string{`
name: System.Upload.Completion
type: CLIENT_EVENT
`, `
name: Generic.Client.Profile
type: CLIENT
`})

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	client_info := &actions_proto.ClientInfo{
		ClientId: self.client_id,
	}
	err = db.SetSubject(self.ConfigObj, client_path_manager.Path(), client_info)
	assert.NoError(self.T(), err)
}

func (self *TestSuite) TestGetFlow() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(
		self.ConfigObj)
	assert.NoError(self.T(), err)

	request1 := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Artifacts: []string{"Generic.Client.Info"},
	}

	request2 := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Artifacts: []string{"Generic.Client.Profile"},
	}

	// Schedule new flows.
	ctx := context.Background()
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	flow_ids := []string{}

	// Create 40 flows with 2 types.
	for i := 0; i < 20; i++ {
		flow_id, err := launcher.ScheduleArtifactCollection(
			ctx, self.ConfigObj,
			vql_subsystem.NullACLManager{},
			repository, request1, nil)
		assert.NoError(self.T(), err)

		flow_ids = append(flow_ids, flow_id)

		flow_id, err = launcher.ScheduleArtifactCollection(
			ctx, self.ConfigObj,
			vql_subsystem.NullACLManager{},
			repository, request2, nil)
		assert.NoError(self.T(), err)

		flow_ids = append(flow_ids, flow_id)
	}

	// Get all the responses - ask for 100 results if available
	// but only 40 are there.
	api_response, err := GetFlows(self.ConfigObj,
		self.client_id, true,
		func(flow *flows_proto.ArtifactCollectorContext) bool {
			return true
		}, 0, 100)
	assert.NoError(self.T(), err)

	// There should be 40 flows (2 sets of each)
	assert.Equal(self.T(), 40, len(api_response.Items))

	// Now only get Generic.Client.Info flows by applying a filter.
	api_response, err = GetFlows(self.ConfigObj,
		self.client_id, true,
		func(flow *flows_proto.ArtifactCollectorContext) bool {
			return flow.Request.Artifacts[0] == "Generic.Client.Info"
		}, 0, 100)
	assert.NoError(self.T(), err)

	// There should be 20 flows of type Generic.Client.Info
	assert.Equal(self.T(), 20, len(api_response.Items))
	for _, item := range api_response.Items {
		assert.Equal(self.T(), "Generic.Client.Info", item.Request.Artifacts[0])
	}
}

func (self *TestSuite) TestRetransmission() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(
		self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Artifacts: []string{"Generic.Client.Info"},
	}

	// Schedule a new flow.
	ctx := context.Background()
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, self.ConfigObj,
		vql_subsystem.NullACLManager{},
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

	runner := NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Retransmit the same row again - this can happen if the
	// server is loaded and the client is re-uploading the same
	// payload multiple times.
	runner = NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Load the collection context and see what happened.
	collection_context, err := LoadCollectionContext(self.ConfigObj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// The flow should have only a single row though.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(1))

}

func (self *TestSuite) TestResourceLimits() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)
	repository, err := manager.GetGlobalRepository(
		self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Artifacts: []string{"Generic.Client.Info"},

		// Only accept 5 rows.
		MaxRows: 5,
	}

	// Schedule a new flow.
	ctx := context.Background()
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx,
		self.ConfigObj,
		vql_subsystem.NullACLManager{},
		repository, request, nil)
	assert.NoError(self.T(), err)

	// Drain messages to the client.
	client_info_manager, err := services.GetClientInfoManager()
	assert.NoError(self.T(), err)

	messages, err := client_info_manager.GetClientTasks(self.client_id)
	assert.NoError(self.T(), err)

	// Two requests since there are two source preconditions on
	// Generic.Client.Info
	assert.Equal(self.T(), len(messages), 2)

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

	runner := NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Load the collection context and see what happened.
	collection_context, err := LoadCollectionContext(self.ConfigObj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// Collection has 1 row and it is still in the running state.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(1))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_RUNNING)

	// Send another row
	message.ResponseId++
	runner = NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Load the collection context and see what happened.
	collection_context, err = LoadCollectionContext(self.ConfigObj,
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
	runner = NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Load the collection context and see what happened.
	collection_context, err = LoadCollectionContext(self.ConfigObj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// Collection has 7 rows and it is still in the running state.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(7))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_ERROR)

	assert.Contains(self.T(), collection_context.Status, "Row count exceeded")

	// Make sure a cancel message was sent to the client.
	messages, err = client_info_manager.PeekClientTasks(self.client_id)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(messages), 1)
	assert.NotNil(self.T(), messages[0].Cancel)

	// Another message arrives from the client - this happens
	// usually because the client has not received the cancel yet
	// and is already sending the next message in the queue.
	message.ResponseId++
	runner = NewFlowRunner(self.ConfigObj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// We still collect these rows but the flow is still in the
	// error state. We do this so we dont lose the last few
	// messages which are still in flight.
	collection_context, err = LoadCollectionContext(self.ConfigObj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(12))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_ERROR)
}

func (self *TestSuite) TestClientUploaderStoreFile() {
	resp := responder.TestResponder()
	uploader := &uploads.VelociraptorUploader{
		Responder: resp,
	}

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
		Set(vql_subsystem.ACL_MANAGER_VAR, vql_subsystem.NullACLManager{}))
	uploader.Upload(context.Background(), scope,
		filename, "ntfs", "", 1000,
		nilTime, nilTime, nilTime, nilTime, reader)

	// Get a new collection context.
	collection_context := NewCollectionContext(self.ConfigObj)
	collection_context.ArtifactCollectorContext = flows_proto.ArtifactCollectorContext{
		SessionId:           self.flow_id,
		ClientId:            self.client_id,
		OutstandingRequests: 1,
		Request: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Generic.Client.Info"},
		},
	}

	for _, resp := range responder.GetTestResponses(resp) {
		resp.Source = self.client_id
		err := ArtifactCollectorProcessOneMessage(
			self.ConfigObj, collection_context, resp)
		assert.NoError(self.T(), err)
	}

	// Close the context should force uploaded files to be
	// flushed.
	closeContext(self.ConfigObj, collection_context)

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
			flow_path_manager.GetUploadsFile("ntfs", "foo").Path()),
		"Hello world ")

	// Check the upload metadata file.
	upload_metadata_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		flow_path_manager.UploadMetadata())

	assert.Equal(self.T(), len(upload_metadata_rows), 1)

	vfs_path, _ := upload_metadata_rows[0].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "foo").
			Path().AsClientPath())

	file_size, _ := upload_metadata_rows[0].GetInt64("file_size")
	assert.Equal(self.T(), file_size, int64(12))

	uploaded_size, _ := upload_metadata_rows[0].GetInt64("uploaded_size")
	assert.Equal(self.T(), uploaded_size, int64(12))

	// Check the System.Upload.Completion event.
	artifact_path_manager, err := artifacts.NewArtifactPathManager(
		self.ConfigObj, self.client_id, self.flow_id,
		"System.Upload.Completion")
	assert.NoError(self.T(), err)

	event_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		artifact_path_manager.Path())

	assert.Equal(self.T(), len(event_rows), 1)

	vfs_path, _ = event_rows[0].GetString("VFSPath")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "foo").
			Path().AsClientPath())

	file_size, _ = event_rows[0].GetInt64("Size")
	assert.Equal(self.T(), file_size, int64(12))

	uploaded_size, _ = event_rows[0].GetInt64("UploadedSize")
	assert.Equal(self.T(), uploaded_size, int64(12))
}

// Just a normal collection with error log - receive some rows and an
// ok status but an error log. An error does not stop the server from
// receiving more rows - all an error does is mark the collection as
// errored.
func (self *TestSuite) TestCollectionCompletionErrorLogWithOkStatus() {
	flow := self.testCollectionCompletion(1, []*crypto_proto.VeloMessage{
		{
			SessionId: self.flow_id,
			RequestId: 1,
			LogMessage: &crypto_proto.LogMessage{
				Message: "Error: This is an error",
			},
		},
		{
			SessionId: self.flow_id,
			RequestId: 1,
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
			RequestId: 1,
			Status: &crypto_proto.GrrStatus{
				Status:   crypto_proto.GrrStatus_OK,
				Duration: 100,
			},
		},
	})

	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR, flow.State)
	assert.Contains(self.T(), flow.Status, "Error:")
	assert.Equal(self.T(), uint64(1), flow.TotalCollectedRows)
	assert.Equal(self.T(), []string{"Generic.Client.Info/BasicInformation"},
		flow.ArtifactsWithResults)

	// Records the correct execution duration.
	assert.Equal(self.T(), int64(100), flow.ExecutionDuration)
}

// Just a normal collection - receive some rows and an ok status
func (self *TestSuite) TestCollectionCompletionOkStatus() {
	flow := self.testCollectionCompletion(1, []*crypto_proto.VeloMessage{
		{
			SessionId: self.flow_id,
			RequestId: 1,
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
			RequestId: 1,
			Status: &crypto_proto.GrrStatus{
				Status:   crypto_proto.GrrStatus_OK,
				Duration: 100,
			},
		},
	})

	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED,
		flow.State)

	// Records the correct execution duration.
	assert.Equal(self.T(), int64(100), flow.ExecutionDuration)
	assert.Equal(self.T(), uint64(1), flow.TotalCollectedRows)
	assert.Equal(self.T(), []string{"Generic.Client.Info/BasicInformation"},
		flow.ArtifactsWithResults)
}

// Emulate an artifact with 2 sources - both responses come one after
// the other but second query failed - this means the entire
// collection is failed but we get all the results.
func (self *TestSuite) TestCollectionCompletionSuccessFollowedByErrLog() {
	flow := self.testCollectionCompletion(2,
		[]*crypto_proto.VeloMessage{
			{
				SessionId: self.flow_id,
				RequestId: 1,
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
				RequestId: 1,
				Status: &crypto_proto.GrrStatus{
					Status:   crypto_proto.GrrStatus_OK,
					Duration: 100,
				},
			},
			{
				SessionId: self.flow_id,
				RequestId: 1,
				VQLResponse: &actions_proto.VQLResponse{
					JSONLResponse: "{}",
					TotalRows:     1,
					Query: &actions_proto.VQLRequest{
						Name: "Generic.Client.Info/Users",
					},
				},
			},
			{
				SessionId: self.flow_id,
				RequestId: 1,
				LogMessage: &crypto_proto.LogMessage{
					Message: "Error: This is an error",
				},
			},
			{
				SessionId: self.flow_id,
				RequestId: 1,
				Status: &crypto_proto.GrrStatus{
					Status:   crypto_proto.GrrStatus_OK,
					Duration: 200,
				},
			},
		})

	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR, flow.State)
	assert.Contains(self.T(), flow.Status, "Error:")
	assert.Equal(self.T(), uint64(2), flow.TotalCollectedRows)
	assert.Equal(self.T(), []string{
		"Generic.Client.Info/BasicInformation",
		"Generic.Client.Info/Users",
	}, flow.ArtifactsWithResults)

	// Records the correct execution duration.
	assert.Equal(self.T(), int64(300), flow.ExecutionDuration)
}

// Emulate an artifact with 2 sources - a single response arrives with
// error - error flow is sent immediately before second response
// arrives.
func (self *TestSuite) TestCollectionCompletionTwoSourcesIncomplete() {
	flow := self.testCollectionCompletion(2,
		[]*crypto_proto.VeloMessage{
			{
				SessionId: self.flow_id,
				RequestId: 1,
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
				RequestId: 1,
				Status: &crypto_proto.GrrStatus{
					Status:   crypto_proto.GrrStatus_GENERIC_ERROR,
					Duration: 100,
				},
			},
		})

	// Collection is still running
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_ERROR, flow.State)
	assert.Equal(self.T(), uint64(1), flow.TotalCollectedRows)
	assert.Equal(self.T(), []string{
		"Generic.Client.Info/BasicInformation",
	}, flow.ArtifactsWithResults)

	// Records the correct execution duration.
	assert.Equal(self.T(), int64(100), flow.ExecutionDuration)
}

func (self *TestSuite) testCollectionCompletion(
	outstanding_requests int64,
	requests []*crypto_proto.VeloMessage) *flows_proto.ArtifactCollectorContext {
	// Get a new collection context.
	collection_context := NewCollectionContext(self.ConfigObj)
	collection_context.ArtifactCollectorContext = flows_proto.ArtifactCollectorContext{
		SessionId:           self.flow_id,
		ClientId:            self.client_id,
		State:               flows_proto.ArtifactCollectorContext_RUNNING,
		OutstandingRequests: outstanding_requests,
		Request: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"Generic.Client.Info"},
		},
	}

	runner := NewFlowRunner(self.ConfigObj)
	runner.context_map[self.flow_id] = collection_context

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}
	rows := []*ordereddict.Dict{}

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

	// Emulate sending an error log followed by a success status. This
	// should still be detected as an error and the flow should be
	// marked as errored.
	for _, resp := range requests {
		runner.ProcessSingleMessage(self.Ctx, resp)
	}

	runner.Close()

	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		mu.Lock()
		defer mu.Unlock()

		return len(rows) > 0
	})

	flow_any, pres := rows[0].Get("Flow")
	assert.True(self.T(), pres)

	flow, ok := flow_any.(*flows_proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	return flow
}

func (self *TestSuite) TestClientUploaderStoreSparseFile() {
	resp := responder.TestResponder()
	uploader := &uploads.VelociraptorUploader{
		Responder: resp,
	}

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
		Set(vql_subsystem.ACL_MANAGER_VAR, vql_subsystem.NullACLManager{}))
	uploader.Upload(context.Background(), scope,
		sparse_filename, "ntfs", "", 1000,
		nilTime, nilTime, nilTime, nilTime, reader)

	// Get a new collection context.
	collection_context := NewCollectionContext(self.ConfigObj)
	collection_context.ArtifactCollectorContext = flows_proto.ArtifactCollectorContext{
		SessionId:           self.flow_id,
		ClientId:            self.client_id,
		OutstandingRequests: 1,
		Request:             &flows_proto.ArtifactCollectorArgs{},
	}

	for _, resp := range responder.GetTestResponses(resp) {
		resp.Source = self.client_id

		// The uploader should be telling us the overall stats.
		assert.Equal(self.T(), resp.FileBuffer.Size, uint64(18))
		assert.Equal(self.T(), resp.FileBuffer.StoredSize, uint64(12))

		ArtifactCollectorProcessOneMessage(self.ConfigObj,
			collection_context, resp)
	}

	// Close the context should force uploaded files to be
	// flushed.
	closeContext(self.ConfigObj, collection_context)

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
			flow_path_manager.GetUploadsFile("ntfs", "sparse").Path()),
		"Hello hello ")

	// Check the upload metadata file.
	upload_metadata_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		flow_path_manager.UploadMetadata())

	// There should be two rows - one for the file and one for the index.
	assert.Equal(self.T(), len(upload_metadata_rows), 2)

	vfs_path, _ := upload_metadata_rows[0].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").
			Path().AsClientPath())

	// The file is actually 18 bytes on the client.
	file_size, _ := upload_metadata_rows[0].GetInt64("file_size")
	assert.Equal(self.T(), file_size, int64(18))

	// But we have 12 bytes in total uploaded
	uploaded_size, _ := upload_metadata_rows[0].GetInt64("uploaded_size")
	assert.Equal(self.T(), uploaded_size, int64(12))

	// Second row is for the index.
	vfs_path, _ = upload_metadata_rows[1].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").
			IndexPath().AsClientPath())

	// Check the System.Upload.Completion event.
	artifact_path_manager, err := artifacts.NewArtifactPathManager(
		self.ConfigObj, self.client_id, self.flow_id,
		"System.Upload.Completion")
	assert.NoError(self.T(), err)

	event_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		artifact_path_manager.Path())

	assert.Equal(self.T(), len(event_rows), 1)

	vfs_path, _ = event_rows[0].GetString("VFSPath")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").
			Path().AsClientPath())

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
		Set(vql_subsystem.ACL_MANAGER_VAR, vql_subsystem.NullACLManager{}))
	accessor, err := accessors.GetAccessor("ntfs", scope)
	assert.NoError(self.T(), err)

	fd, err := accessor.Open(filename)
	assert.NoError(self.T(), err)

	resp := responder.TestResponder()
	uploader := &uploads.VelociraptorUploader{
		Responder: resp,
	}

	// Upload the file to the responder.
	uploader.Upload(context.Background(), scope,
		sparse_filename, "ntfs", "", 1000,
		nilTime, nilTime, nilTime, nilTime, fd)

	// Get a new collection context.
	collection_context := NewCollectionContext(self.ConfigObj)
	collection_context.ArtifactCollectorContext = flows_proto.ArtifactCollectorContext{
		SessionId: self.flow_id,
		ClientId:  self.client_id,
		Request:   &flows_proto.ArtifactCollectorArgs{},
	}

	// Process it.
	for _, resp := range responder.GetTestResponses(resp) {
		resp.Source = self.client_id
		ArtifactCollectorProcessOneMessage(self.ConfigObj,
			collection_context, resp)
	}

	// Close the context should force uploaded files to be
	// flushed.
	closeContext(self.ConfigObj, collection_context)

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
			flow_path_manager.GetUploadsFile("ntfs", "sparse").Path()),
		"")

	// Check the upload metadata file.
	upload_metadata_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		flow_path_manager.UploadMetadata())

	// There should be two rows - one for the file and one for the index.
	assert.Equal(self.T(), len(upload_metadata_rows), 2)

	vfs_path, _ := upload_metadata_rows[0].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").
			Path().AsClientPath())

	// The file is actually 0x100000 bytes on the client.
	file_size, _ := upload_metadata_rows[0].GetInt64("file_size")
	assert.Equal(self.T(), file_size, int64(0x100000))

	// But we have 0 bytes in total uploaded because the entire file is sparse
	uploaded_size, _ := upload_metadata_rows[0].GetInt64("uploaded_size")
	assert.Equal(self.T(), uploaded_size, int64(0))

	// Second row is for the index.
	vfs_path, _ = upload_metadata_rows[1].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").
			IndexPath().AsClientPath())

	// Check the System.Upload.Completion event.
	artifact_path_manager, err := artifacts.NewArtifactPathManager(
		self.ConfigObj, self.client_id, self.flow_id,
		"System.Upload.Completion")
	assert.NoError(self.T(), err)

	event_rows := test_utils.FileReadRows(self.T(), self.ConfigObj,
		artifact_path_manager.Path())

	assert.Equal(self.T(), len(event_rows), 1)

	vfs_path, _ = event_rows[0].GetString("VFSPath")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").
			Path().AsClientPath())

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
