package vfs_service

import (
	"path"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/flows/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

var definitions = []string{`
name: System.VFS.ListDirectory
`, `
name: System.VFS.DownloadFile
`,
}

type VFSServiceTestSuite struct {
	test_utils.TestSuite

	client_id string
	flow_id   string
}

func (self *VFSServiceTestSuite) SetupTest() {
	self.TestSuite.SetupTest()

	require.NoError(self.T(), self.Sm.Start(StartVFSService))
	self.LoadArtifacts(definitions)

	self.client_id = "C.12312"
	self.flow_id = "F.1232"
}

func (self *VFSServiceTestSuite) EmulateCollection(
	artifact string, rows []*ordereddict.Dict) string {

	// Emulate a Generic.Client.Info collection: First write the
	// result set, then write the collection context.
	journal, err := services.GetJournal()
	assert.NoError(self.T(), err)

	journal.PushRowsToArtifact(self.ConfigObj, rows,
		artifact, self.client_id, self.flow_id)

	// Emulate a flow completion message coming from the flow processor.
	journal.PushRowsToArtifact(self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", self.client_id).
			Set("FlowId", self.flow_id).
			Set("Flow", &flows_proto.ArtifactCollectorContext{
				ClientId:             self.client_id,
				SessionId:            self.flow_id,
				ArtifactsWithResults: []string{artifact},
				TotalCollectedRows:   uint64(len(rows)),
			})},
		"System.Flow.Completion", "server", "")

	return self.flow_id
}

func (self *VFSServiceTestSuite) TestVFSListDirectory() {
	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "c"),
			makeStat("/a/b", "d"),
			makeStat("/a/b", "e"),
		})

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	resp := &api_proto.VFSListResponse{}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.ConfigObj,
			client_path_manager.VFSPath([]string{"file", "a", "b"}),
			resp)
		return resp.TotalRows == 3
	})
	assert.Equal(self.T(), self.getFullPath(resp), []string{
		"/a/b/c", "/a/b/d", "/a/b/e",
	})
}

func (self *VFSServiceTestSuite) TestVFSListDirectoryEmpty() {
	journal, err := services.GetJournal()
	assert.NoError(self.T(), err)

	// Emulate a flow completion message coming from the flow processor.
	artifact := "System.VFS.ListDirectory"
	journal.PushRowsToArtifact(self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", self.client_id).
			Set("FlowId", self.flow_id).
			Set("Flow", &flows_proto.ArtifactCollectorContext{
				ClientId:  self.client_id,
				SessionId: self.flow_id,
				Request: &flows_proto.ArtifactCollectorArgs{
					Artifacts: []string{artifact},
					Specs: []*flows_proto.ArtifactSpec{{
						Artifact: artifact,
						Parameters: &flows_proto.ArtifactParameters{
							Env: []*actions_proto.VQLEnv{{
								Key:   "Path",
								Value: "/a/b/",
							}, {
								Key:   "Accessor",
								Value: "file",
							}},
						},
					}},
				}}),
		}, "System.Flow.Completion", "server", "")

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	resp := &api_proto.VFSListResponse{}

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.ConfigObj,
			client_path_manager.VFSPath([]string{"file", "a", "b"}),
			resp)
		return resp.Timestamp > 0
	})
	assert.Equal(self.T(), self.getFullPath(resp), []string{})
}

func (self *VFSServiceTestSuite) TestRecursiveVFSListDirectory() {
	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "A"),
			makeStat("/a/b", "B"),
			makeStat("/a/b/c", "CA"),
			makeStat("/a/b/c", "CB"),
		})

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	resp := &api_proto.VFSListResponse{}

	// The response in VFS path /file/a/b
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.ConfigObj,
			client_path_manager.VFSPath([]string{"file", "a", "b"}),
			resp)
		return resp.TotalRows == 2
	})

	assert.Equal(self.T(), self.getFullPath(resp), []string{
		"/a/b/A", "/a/b/B",
	})

	resp = &api_proto.VFSListResponse{}

	// The response in VFS path /file/a/b/c
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.ConfigObj,
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
	client_path_manager := paths.NewClientPathManager(self.client_id)

	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "A"),
			makeStat("/a/b", "B"),
		})

	// Simulate an upload that was received by our System.VFS.DownloadFile collection.
	file_store := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	fd, err := file_store.WriteFile(flow_path_manager.GetUploadsFile("file", "/a/b/B").Path())
	assert.NoError(self.T(), err)
	fd.Write([]byte("Data"))
	fd.Close()

	self.EmulateCollection(
		"System.VFS.DownloadFile", []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("Path", "/a/b/B").
				Set("Accessor", "file").
				Set("Size", 10),
		})

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	// The VFS service stores a file in the VFS area of the
	// client's namespace pointing to the real data. The real data
	// is stored in the collection's space.
	resp := &proto.VFSDownloadInfo{}
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.ConfigObj,
			client_path_manager.VFSDownloadInfoFromClientPath(
				"file", "/a/b/B"),
			resp)
		return resp.Size == 10
	})

	assert.Equal(self.T(), resp.Components,
		flow_path_manager.GetUploadsFile("file", "/a/b/B").
			Path().Components())
}

func (self *VFSServiceTestSuite) getFullPath(resp *api_proto.VFSListResponse) []string {
	json_response := resp.Response
	rows, err := utils.ParseJsonToDicts([]byte(json_response))
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
