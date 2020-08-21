package vfs_service

import (
	"context"
	"path"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/memory"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/flows/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/services/notifications"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
)

type VFSServiceTestSuite struct {
	suite.Suite
	config_obj *config_proto.Config
	ctx        context.Context
	client_id  string
	flow_id    string
	sm         *services.Service
}

func (self *VFSServiceTestSuite) GetMemoryFileStore() *memory.MemoryFileStore {
	file_store_factory, ok := file_store.GetFileStore(
		self.config_obj).(*memory.MemoryFileStore)
	require.True(self.T(), ok)

	return file_store_factory
}

func (self *VFSServiceTestSuite) SetupTest() {
	var err error
	self.config_obj, err = new(config.Loader).WithFileLoader(
		"../../http_comms/test_data/server.config.yaml").
		WithRequiredFrontend().WithWriteback().
		LoadAndValidate()
	require.NoError(self.T(), err)

	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	self.sm = services.NewServiceManager(ctx, self.config_obj)

	require.NoError(self.T(), self.sm.Start(journal.StartJournalService))
	require.NoError(self.T(), self.sm.Start(launcher.StartLauncherService))
	require.NoError(self.T(), self.sm.Start(notifications.StartNotificationService))
	require.NoError(self.T(), self.sm.Start(repository.StartRepositoryManager))
	require.NoError(self.T(), self.sm.Start(StartVFSService))

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
}

func (self *VFSServiceTestSuite) TearDownTest() {
	// Reset the data store.
	db, err := datastore.GetDB(self.config_obj)
	require.NoError(self.T(), err)

	db.Close()
	test_utils.GetMemoryFileStore(self.T(), self.config_obj).Clear()
	self.sm.Close()
}

func (self *VFSServiceTestSuite) EmulateCollection(
	artifact string, rows []*ordereddict.Dict) string {

	// Emulate a Generic.Client.Info collection: First write the
	// result set, then write the collection context.
	artifact_path_manager := result_sets.NewArtifactPathManager(
		self.config_obj, self.client_id, self.flow_id, artifact)

	// Write a result set for this artifact.
	services.GetJournal().PushRows(artifact_path_manager, rows)

	// Emulate a flow completion message coming from the flow processor.
	artifact_path_manager = result_sets.NewArtifactPathManager(
		self.config_obj, "server", "", "System.Flow.Completion")

	services.GetJournal().PushRows(artifact_path_manager,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", self.client_id).
			Set("FlowId", self.flow_id).
			Set("Flow", &flows_proto.ArtifactCollectorContext{
				ClientId:             self.client_id,
				SessionId:            self.flow_id,
				ArtifactsWithResults: []string{artifact}})})
	return self.flow_id
}

func (self *VFSServiceTestSuite) TestVFSListDirectory() {
	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "c"),
			makeStat("/a/b", "d"),
			makeStat("/a/b", "e"),
		})

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	resp := &flows_proto.VFSListResponse{}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj,
			client_path_manager.VFSPath([]string{"file", "a", "b"}),
			resp)
		return resp.TotalRows == 3
	})
	assert.Equal(self.T(), self.getFullPath(resp), []string{
		"/a/b/c", "/a/b/d", "/a/b/e",
	})
}

func (self *VFSServiceTestSuite) TestRecursiveVFSListDirectory() {
	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "A"),
			makeStat("/a/b", "B"),
			makeStat("/a/b/c", "CA"),
			makeStat("/a/b/c", "CB"),
		})

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	resp := &flows_proto.VFSListResponse{}

	// The response in VFS path /file/a/b
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj,
			client_path_manager.VFSPath([]string{"file", "a", "b"}),
			resp)
		return resp.TotalRows == 2
	})

	assert.Equal(self.T(), self.getFullPath(resp), []string{
		"/a/b/A", "/a/b/B",
	})

	// The response in VFS path /file/a/b/c
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj,
			client_path_manager.VFSPath([]string{"file", "a", "b", "c"}),
			resp)
		return resp.TotalRows == 2
	})

	assert.Equal(self.T(), self.getFullPath(resp), []string{
		"/a/b/c/CA", "/a/b/c/CB",
	})
}

func (self *VFSServiceTestSuite) TestVFSDownload() {
	flow_path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id)

	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "A"),
			makeStat("/a/b", "B"),
		})

	// Simulate and upload was received by our System.VFS.DownloadFile collection.
	file_store := self.GetMemoryFileStore()
	file_store.Data[flow_path_manager.GetUploadsFile("file", "/a/b/B").Path()] = []byte("Data")

	self.EmulateCollection(
		"System.VFS.DownloadFile", []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("Path", "/a/b/B").
				Set("Accessor", "file").
				Set("Size", 10),
		})

	db, err := datastore.GetDB(self.config_obj)
	assert.NoError(self.T(), err)

	// The VFS service stores a file in the VFS area of the
	// client's namespace pointing to the real data. The real data
	// is stored in the collection's space.
	resp := &proto.VFSDownloadInfo{}
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.config_obj,
			flow_path_manager.GetVFSDownloadInfoPath("file", "/a/b/B").Path(),
			resp)
		return resp.Size == 10
	})

	assert.Equal(self.T(), resp.VfsPath,
		flow_path_manager.GetUploadsFile("file", "/a/b/B").Path())
}

func (self *VFSServiceTestSuite) getFullPath(resp *flows_proto.VFSListResponse) []string {
	rows, err := utils.ParseJsonToDicts([]byte(resp.Response))
	assert.NoError(self.T(), err)

	result := []string{}
	for _, row := range rows {
		full_path, ok := row.GetString("_FullPath")
		if ok {
			result = append(result, full_path)
		}
	}

	return result
}

func makeStat(dirname, name string) *ordereddict.Dict {
	fullpath := path.Join(dirname, name)
	return ordereddict.NewDict().Set("_FullPath", fullpath).
		Set("Name", name).Set("_Accessor", "file")
}

func TestVFSService(t *testing.T) {
	suite.Run(t, &VFSServiceTestSuite{})
}
