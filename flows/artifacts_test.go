package flows

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/inventory"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"

	_ "www.velocidex.com/golang/velociraptor/vql_plugins"
)

type TestRangeReader struct {
	*bytes.Reader
	ranges []uploads.Range
}

func (self *TestRangeReader) Ranges() []uploads.Range {
	return self.ranges
}

type TestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	client_id  string
	flow_id    string
	sm         *services.Service
}

func (self *TestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	self.config_obj.Frontend.DoNotCompressArtifacts = true

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(inventory.StartInventoryService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
}

func (self *TestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
}

func (self *TestSuite) TestGetFlow() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(
		self.config_obj)
	assert.NoError(self.T(), err)

	request1 := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Artifacts: []string{"Generic.Client.Info"},
	}

	request2 := &flows_proto.ArtifactCollectorArgs{
		ClientId:  self.client_id,
		Artifacts: []string{"Generic.Client.Profile"},
	}

	// Schedule a new flow.
	ctx := context.Background()
	launcher, err := services.GetLauncher()
	assert.NoError(self.T(), err)

	flow_ids := []string{}

	// Create 40 flows with 2 types.
	for i := 0; i < 20; i++ {
		flow_id, err := launcher.ScheduleArtifactCollection(
			ctx, self.config_obj,
			vql_subsystem.NullACLManager{},
			repository, request1)
		assert.NoError(self.T(), err)

		flow_ids = append(flow_ids, flow_id)

		flow_id, err = launcher.ScheduleArtifactCollection(
			ctx, self.config_obj,
			vql_subsystem.NullACLManager{},
			repository, request2)
		assert.NoError(self.T(), err)

		flow_ids = append(flow_ids, flow_id)
	}

	// Get all the responses - ask for 100 results if available
	// but only 40 are there.
	api_response, err := GetFlows(self.config_obj,
		self.client_id, true,
		func(flow *flows_proto.ArtifactCollectorContext) bool {
			return true
		}, 0, 100)
	assert.NoError(self.T(), err)

	// There should be 40 flows (2 sets of each)
	assert.Equal(self.T(), 40, len(api_response.Items))

	// Now only get Generic.Client.Info flows by applying a filter.
	api_response, err = GetFlows(self.config_obj,
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

	// Page the response now - only ask for first 10 flows.
	api_response_subset, err := GetFlows(self.config_obj,
		self.client_id, true,
		func(flow *flows_proto.ArtifactCollectorContext) bool {
			return flow.Request.Artifacts[0] == "Generic.Client.Info"
		}, 0, 10)
	assert.NoError(self.T(), err)

	// These should be the same order as the entire result
	assert.Equal(self.T(), api_response.Items[:10], api_response_subset.Items)

	// Next page
	api_response_subset, err = GetFlows(self.config_obj,
		self.client_id, true,
		func(flow *flows_proto.ArtifactCollectorContext) bool {
			return flow.Request.Artifacts[0] == "Generic.Client.Info"
		}, 10, 10)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), api_response.Items[10:20], api_response_subset.Items)

	// Now test GetFlows's ability to stitch results from multiple
	// calls to datastore ListChildren(). Set
	// get_flows_sub_query_count to a small value.
	get_flows_sub_query_count = 5

	api_response_small_backend, err := GetFlows(self.config_obj,
		self.client_id, true,
		func(flow *flows_proto.ArtifactCollectorContext) bool {
			return flow.Request.Artifacts[0] == "Generic.Client.Info"
		}, 0, 100)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), json.StringIndent(api_response.Items),
		json.StringIndent(api_response_small_backend.Items))
}

func (self *TestSuite) TestRetransmission() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(
		self.config_obj)
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
		ctx, self.config_obj,
		vql_subsystem.NullACLManager{},
		repository, request)
	assert.NoError(self.T(), err)

	// Send one row.
	message := &crypto_proto.GrrMessage{
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

	runner := NewFlowRunner(self.config_obj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Retransmit the same row again - this can happen if the
	// server is loaded and the client is re-uploading the same
	// payload multiple times.
	runner = NewFlowRunner(self.config_obj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Load the collection context and see what happened.
	collection_context, err := LoadCollectionContext(self.config_obj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// The flow should have only a single row though.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(1))

}

func (self *TestSuite) TestResourceLimits() {
	manager, err := services.GetRepositoryManager()
	assert.NoError(self.T(), err)
	repository, err := manager.GetGlobalRepository(
		self.config_obj)
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
		self.config_obj,
		vql_subsystem.NullACLManager{},
		repository, request)
	assert.NoError(self.T(), err)

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	// Drain messages to the client.
	messages, err := db.GetClientTasks(self.config_obj, self.client_id,
		false /* do_not_lease */)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(messages), 1)

	// Send one row.
	message := &crypto_proto.GrrMessage{
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

	runner := NewFlowRunner(self.config_obj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Load the collection context and see what happened.
	collection_context, err := LoadCollectionContext(self.config_obj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// Collection has 1 row and it is still in the running state.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(1))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_RUNNING)

	// Send another row
	message.ResponseId++
	runner = NewFlowRunner(self.config_obj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Load the collection context and see what happened.
	collection_context, err = LoadCollectionContext(self.config_obj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// Collection has 1 row and it is still in the running state.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(2))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_RUNNING)

	// Now send 5 rows in one message. We should accept the 5 rows
	// but terminate the flow due to resource exhaustion.
	message.VQLResponse.TotalRows = 5
	message.ResponseId++
	runner = NewFlowRunner(self.config_obj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// Load the collection context and see what happened.
	collection_context, err = LoadCollectionContext(self.config_obj,
		self.client_id, flow_id)
	assert.NoError(self.T(), err)

	// Collection has 1 row and it is still in the running state.
	assert.Equal(self.T(), collection_context.TotalCollectedRows, uint64(7))
	assert.Equal(self.T(), collection_context.State,
		flows_proto.ArtifactCollectorContext_ERROR)

	assert.Contains(self.T(), collection_context.Status, "Row count exceeded")

	// Make sure a cancel message was sent to the client.
	messages, err = db.GetClientTasks(self.config_obj, self.client_id,
		false /* do_not_lease */)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), len(messages), 1)
	assert.NotNil(self.T(), messages[0].Cancel)

	// Another message arrives from the client - this happens
	// usually because the client has not received the cancel yet
	// and is already sending the next message in the queue.
	message.ResponseId++
	runner = NewFlowRunner(self.config_obj)
	runner.ProcessSingleMessage(ctx, message)
	runner.Close()

	// We still collect these rows but the flow is still in the
	// error state. We do this so we dont lose the last few
	// messages which are still in flight.
	collection_context, err = LoadCollectionContext(self.config_obj,
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

	scope := vql_subsystem.MakeScope()
	uploader.Upload(context.Background(), scope,
		"foo", "ntfs", "", 1000, reader)

	// Get a new collection context.
	collection_context := &flows_proto.ArtifactCollectorContext{
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
			self.config_obj, collection_context, resp)
		assert.NoError(self.T(), err)
	}

	// Close the context should force uploaded files to be
	// flushed.
	closeContext(self.config_obj, collection_context)

	assert.Equal(self.T(), collection_context.TotalUploadedFiles, uint64(1))

	// Total bytes actually delivered and expected.
	assert.Equal(self.T(), collection_context.TotalUploadedBytes, uint64(12))
	assert.Equal(self.T(), collection_context.TotalExpectedUploadedBytes, uint64(12))

	// Debug the entire filestore
	// test_utils.GetMemoryFileStore(self.T(), self.config_obj).Debug()

	// Check the file content is there
	flow_path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id)
	assert.Equal(self.T(),
		test_utils.FileReadAll(self.T(), self.config_obj,
			flow_path_manager.GetUploadsFile("ntfs", "foo").Path()),
		"Hello world ")

	// Check the upload metadata file.
	upload_metadata_rows := test_utils.FileReadRows(self.T(), self.config_obj,
		flow_path_manager.UploadMetadata().Path())

	assert.Equal(self.T(), len(upload_metadata_rows), 1)

	vfs_path, _ := upload_metadata_rows[0].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "foo").Path())

	file_size, _ := upload_metadata_rows[0].GetInt64("file_size")
	assert.Equal(self.T(), file_size, int64(12))

	uploaded_size, _ := upload_metadata_rows[0].GetInt64("uploaded_size")
	assert.Equal(self.T(), uploaded_size, int64(12))

	// Check the System.Upload.Completion event.
	artifact_path_manager := artifacts.NewArtifactPathManager(
		self.config_obj, self.client_id, self.flow_id,
		"System.Upload.Completion")

	event_rows := test_utils.FileReadRows(self.T(), self.config_obj,
		artifact_path_manager.Path())

	assert.Equal(self.T(), len(event_rows), 1)

	vfs_path, _ = event_rows[0].GetString("VFSPath")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "foo").Path())

	file_size, _ = event_rows[0].GetInt64("Size")
	assert.Equal(self.T(), file_size, int64(12))

	uploaded_size, _ = event_rows[0].GetInt64("UploadedSize")
	assert.Equal(self.T(), uploaded_size, int64(12))
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

	scope := vql_subsystem.MakeScope()
	uploader.Upload(context.Background(), scope,
		"sparse", "ntfs", "", 1000, reader)

	// Get a new collection context.
	collection_context := &flows_proto.ArtifactCollectorContext{
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

		ArtifactCollectorProcessOneMessage(self.config_obj,
			collection_context, resp)
	}

	// Close the context should force uploaded files to be
	// flushed.
	closeContext(self.config_obj, collection_context)

	// One file is uploaded
	assert.Equal(self.T(), collection_context.TotalUploadedFiles, uint64(1))

	// Total bytes actually delivered and expected.
	assert.Equal(self.T(), collection_context.TotalUploadedBytes, uint64(12))
	assert.Equal(self.T(), collection_context.TotalExpectedUploadedBytes, uint64(12))

	// Debug the entire filestore
	// test_utils.GetMemoryFileStore(self.T(), self.config_obj).Debug()

	// Check the file content is there
	flow_path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id)
	assert.Equal(self.T(),
		test_utils.FileReadAll(self.T(), self.config_obj,
			flow_path_manager.GetUploadsFile("ntfs", "sparse").Path()),
		"Hello hello ")

	// Check the upload metadata file.
	upload_metadata_rows := test_utils.FileReadRows(self.T(), self.config_obj,
		flow_path_manager.UploadMetadata().Path())

	// There should be two rows - one for the file and one for the index.
	assert.Equal(self.T(), len(upload_metadata_rows), 2)

	vfs_path, _ := upload_metadata_rows[0].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").Path())

	// The file is actually 18 bytes on the client.
	file_size, _ := upload_metadata_rows[0].GetInt64("file_size")
	assert.Equal(self.T(), file_size, int64(18))

	// But we have 12 bytes in total uploaded
	uploaded_size, _ := upload_metadata_rows[0].GetInt64("uploaded_size")
	assert.Equal(self.T(), uploaded_size, int64(12))

	// Second row is for the index.
	vfs_path, _ = upload_metadata_rows[1].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").IndexPath())

	// Check the System.Upload.Completion event.
	artifact_path_manager := artifacts.NewArtifactPathManager(
		self.config_obj, self.client_id, self.flow_id,
		"System.Upload.Completion")

	event_rows := test_utils.FileReadRows(self.T(), self.config_obj,
		artifact_path_manager.Path())

	assert.Equal(self.T(), len(event_rows), 1)

	vfs_path, _ = event_rows[0].GetString("VFSPath")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").Path())

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

	scope := vql_subsystem.MakeScope()
	accessor, err := glob.GetAccessor("ntfs", scope)
	assert.NoError(self.T(), err)

	fd, err := accessor.Open(filename)
	assert.NoError(self.T(), err)

	resp := responder.TestResponder()
	uploader := &uploads.VelociraptorUploader{
		Responder: resp,
	}

	// Upload the file to the responder.
	uploader.Upload(context.Background(), scope,
		"sparse", "ntfs", "", 1000, fd)

	// Get a new collection context.
	collection_context := &flows_proto.ArtifactCollectorContext{
		SessionId: self.flow_id,
		ClientId:  self.client_id,
		Request:   &flows_proto.ArtifactCollectorArgs{},
	}

	// Process it.
	for _, resp := range responder.GetTestResponses(resp) {
		resp.Source = self.client_id
		ArtifactCollectorProcessOneMessage(self.config_obj,
			collection_context, resp)
	}

	// Close the context should force uploaded files to be
	// flushed.
	closeContext(self.config_obj, collection_context)

	// One file is uploaded
	assert.Equal(self.T(), collection_context.TotalUploadedFiles, uint64(1))

	// Total bytes actually delivered and expected is 0 because the file is sparse.
	assert.Equal(self.T(), collection_context.TotalUploadedBytes, uint64(0))
	assert.Equal(self.T(), collection_context.TotalExpectedUploadedBytes, uint64(0))

	// Debug the entire filestore
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Debug()

	// Check the file content is there
	flow_path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id)
	assert.Equal(self.T(),
		test_utils.FileReadAll(self.T(), self.config_obj,
			flow_path_manager.GetUploadsFile("ntfs", "sparse").Path()),
		"")

	// Check the upload metadata file.
	upload_metadata_rows := test_utils.FileReadRows(self.T(), self.config_obj,
		flow_path_manager.UploadMetadata().Path())

	// There should be two rows - one for the file and one for the index.
	assert.Equal(self.T(), len(upload_metadata_rows), 2)

	vfs_path, _ := upload_metadata_rows[0].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").Path())

	// The file is actually 0x100000 bytes on the client.
	file_size, _ := upload_metadata_rows[0].GetInt64("file_size")
	assert.Equal(self.T(), file_size, int64(0x100000))

	// But we have 0 bytes in total uploaded because the entire file is sparse
	uploaded_size, _ := upload_metadata_rows[0].GetInt64("uploaded_size")
	assert.Equal(self.T(), uploaded_size, int64(0))

	// Second row is for the index.
	vfs_path, _ = upload_metadata_rows[1].GetString("vfs_path")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").IndexPath())

	// Check the System.Upload.Completion event.
	artifact_path_manager := artifacts.NewArtifactPathManager(
		self.config_obj, self.client_id, self.flow_id,
		"System.Upload.Completion")

	event_rows := test_utils.FileReadRows(self.T(), self.config_obj,
		artifact_path_manager.Path())

	assert.Equal(self.T(), len(event_rows), 1)

	vfs_path, _ = event_rows[0].GetString("VFSPath")
	assert.Equal(self.T(), vfs_path,
		flow_path_manager.GetUploadsFile("ntfs", "sparse").Path())

	file_size, _ = event_rows[0].GetInt64("Size")
	assert.Equal(self.T(), file_size, int64(0x100000))

	uploaded_size, _ = event_rows[0].GetInt64("UploadedSize")
	assert.Equal(self.T(), uploaded_size, int64(0))
}

func TestArtifactCollection(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
