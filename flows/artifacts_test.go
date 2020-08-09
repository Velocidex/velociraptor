package flows

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/responder"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
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

	// Start essential services.
	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(services.StartNotificationService))

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
}

func (self *TestSuite) TearDownTest() {
	self.sm.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	test_utils.GetMemoryDataStore(self.T(), self.config_obj).Clear()
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
		SessionId: self.flow_id,
		ClientId:  self.client_id,
	}

	for _, resp := range responder.GetTestResponses(resp) {
		resp.Source = self.client_id
		ArtifactCollectorProcessOneMessage(self.config_obj,
			collection_context, resp)
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
	artifact_path_manager := result_sets.NewArtifactPathManager(
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
		SessionId: self.flow_id,
		ClientId:  self.client_id,
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
	artifact_path_manager := result_sets.NewArtifactPathManager(
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
	artifact_path_manager := result_sets.NewArtifactPathManager(
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
