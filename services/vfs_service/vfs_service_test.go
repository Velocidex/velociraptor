package vfs_service_test

import (
	"path"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	_ "www.velocidex.com/golang/velociraptor/result_sets/timed"
)

var definitions = []string{`
name: System.VFS.ListDirectory
`, `
name: System.VFS.DownloadFile
`, `
name: System.Flow.Completion
type: CLIENT_EVENT
`,
}

type VFSServiceTestSuite struct {
	test_utils.TestSuite

	client_id string
	flow_id   string

	closer func()
}

func (self *VFSServiceTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.VfsService = true
	self.LoadArtifactsIntoConfig(definitions)

	self.TestSuite.SetupTest()

	self.client_id = "C.12312"
	self.flow_id = "F.1232"

	// Register a user manager that returns the superuser user to skip
	// any ACLs checks. This helps us test the API server to make sure
	// the GUI will present the correct data.
	users.RegisterTestUserManager(
		self.ConfigObj, utils.GetSuperuserName(self.ConfigObj))

	self.closer = utils.MockTime(utils.NewMockClock(time.Unix(1000, 0)))
	self.CreateClient(self.client_id)
}

func (self *VFSServiceTestSuite) TearDownTest() {
	self.TestSuite.TearDownTest()
	self.closer()
}

func (self *VFSServiceTestSuite) EmulateCollection(
	artifact string, rows []*ordereddict.Dict) string {

	// Emulate an artifact collection: First write the
	// result set, then write the collection context.
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj, rows,
		artifact, self.client_id, self.flow_id)

	// Emulate a flow completion message coming from the flow processor.
	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
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

// New clients support the vfs_ls() plugin so we write a new style
// collection.
func (self *VFSServiceTestSuite) EmulateCollectionWithVFSLs(
	artifact string, rows []*ordereddict.Dict,
	stats []*ordereddict.Dict) string {

	// Emulate an artifact collection: First write the
	// result set, then write the collection context.
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj, rows,
		artifact+"/Listing", self.client_id, self.flow_id)

	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj, stats,
		artifact+"/Stats", self.client_id, self.flow_id)

	// Emulate a flow completion message coming from the flow processor.
	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("ClientId", self.client_id).
			Set("FlowId", self.flow_id).
			Set("Flow", &flows_proto.ArtifactCollectorContext{
				ClientId:  self.client_id,
				SessionId: self.flow_id,
				ArtifactsWithResults: []string{
					artifact + "/Listing",
					artifact + "/Stats",
				},
				TotalCollectedRows: uint64(len(rows)),
			})},
		"System.Flow.Completion", "server", "")
	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
	return self.flow_id
}

func (self *VFSServiceTestSuite) TestVFSListDirectoryLegacy() {
	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "c"),
			makeStat("/a/b", "d"),
			makeDirectoryStat("/a/b", "e"),
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

	// The response will store a reference to the original collection
	// spanning the rows that describe this directory. In this case
	// all rows are about this directory.
	assert.Equal(self.T(), resp.TotalRows, uint64(3))
	assert.Equal(self.T(), resp.StartIdx, uint64(0))
	assert.Equal(self.T(), resp.EndIdx, uint64(3))
	assert.Equal(self.T(), resp.ClientId, self.client_id)
	assert.Equal(self.T(), resp.FlowId, self.flow_id)
}

func (self *VFSServiceTestSuite) TestVFSListDirectoryNew() {
	self.EmulateCollectionWithVFSLs(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "c"),
			makeStat("/a/b", "d"),
			makeDirectoryStat("/a/b", "e"),
		},
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("Components", []string{"a", "b"}).
				Set("Accessor", "file").
				Set("Stats", &api_proto.VFSListResponse{
					StartIdx: 0,
					EndIdx:   3,
				}),
		})

	db, err := datastore.GetDB(self.ConfigObj)
	assert.NoError(self.T(), err)

	client_path_manager := paths.NewClientPathManager(self.client_id)
	resp := &api_proto.VFSListResponse{}

	vtesting.WaitUntil(2000*time.Second, self.T(), func() bool {
		db.GetSubject(self.ConfigObj,
			client_path_manager.VFSPath([]string{"file", "a", "b"}),
			resp)
		return resp.TotalRows == 3
	})

	// The response will store a reference to the original collection
	// spanning the rows that describe this directory. In this case
	// all rows are about this directory.
	assert.Equal(self.T(), resp.TotalRows, uint64(3))
	assert.Equal(self.T(), resp.StartIdx, uint64(0))
	assert.Equal(self.T(), resp.EndIdx, uint64(3))
	assert.Equal(self.T(), resp.ClientId, self.client_id)
	assert.Equal(self.T(), resp.FlowId, self.flow_id)
}

func (self *VFSServiceTestSuite) TestVFSListDirectoryEmpty() {
	journal, err := services.GetJournal(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Emulate a flow completion message coming from the flow processor.
	artifact := "System.VFS.ListDirectory"
	journal.PushRowsToArtifact(self.Ctx, self.ConfigObj,
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

	// The response will store a reference to the original collection
	// spanning the rows that describe this directory. In this case
	// all rows are about this directory.
	assert.Equal(self.T(), resp.TotalRows, uint64(0))
	assert.Equal(self.T(), resp.StartIdx, uint64(0))
	assert.Equal(self.T(), resp.EndIdx, uint64(0))
	assert.Equal(self.T(), resp.ClientId, self.client_id)
	assert.Equal(self.T(), resp.FlowId, self.flow_id)
}

func (self *VFSServiceTestSuite) TestRecursiveVFSListDirectoryLegacy() {
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

	// The response will store a reference to the original collection
	// spanning the rows that describe this directory. In this case
	// all rows are about this directory.
	assert.Equal(self.T(), resp.ClientId, self.client_id)
	assert.Equal(self.T(), resp.FlowId, self.flow_id)

	// Directory /a/b contains two files being the first 2 rows in the
	// collection.
	assert.Equal(self.T(), resp.TotalRows, uint64(2))
	assert.Equal(self.T(), resp.StartIdx, uint64(0))
	assert.Equal(self.T(), resp.EndIdx, uint64(2))

	resp = &api_proto.VFSListResponse{}

	// The response in VFS path /file/a/b/c
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		db.GetSubject(self.ConfigObj,
			client_path_manager.VFSPath([]string{"file", "a", "b", "c"}),
			resp)
		return resp.TotalRows == 2
	})

	// Directory /a/b/c contains two files being the last 2 rows in
	// the collection.
	assert.Equal(self.T(), resp.TotalRows, uint64(2))
	assert.Equal(self.T(), resp.StartIdx, uint64(2))
	assert.Equal(self.T(), resp.EndIdx, uint64(4))
}

// Check that the API can read a long recursive list directory.
func (self *VFSServiceTestSuite) TestRecursiveVFSListDirectoryApiAccess() {
	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "A"),
			makeStat("/a/b", "B"),
			makeStat("/a/b", "C"),
			makeStat("/a/b", "D"),
			makeStat("/a/b", "E"),
			makeDirectoryStat("/a/b", "c"),
			makeStat("/a/b/c", "CA"),
			makeStat("/a/b/c", "CB"),
			makeStat("/a/b/c", "CC"),
			makeStat("/a/b/c", "CD"),
			makeStat("/a/b/c", "CE"),
			makeStat("/a/b/c", "CF"),
		})

	vfs_service, err := services.GetVFSService(self.ConfigObj)
	assert.NoError(self.T(), err)

	stat := &api_proto.VFSListResponse{}

	// The response in VFS path /file/a/b
	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		stat, err = vfs_service.StatDirectory(self.ConfigObj,
			self.client_id, []string{"file", "a", "b", "c"})
		return err == nil && stat.TotalRows > 0
	})

	golden := ordereddict.NewDict()

	// Check that the GUI can read the data correctly via the API.
	api_service := &api.ApiServer{}
	table, err := api_service.VFSListDirectoryFiles(self.Ctx,
		&api_proto.GetTableRequest{
			ClientId:      stat.ClientId,
			FlowId:        stat.FlowId,
			VfsComponents: []string{"file", "a", "b"},
			Rows:          1000,
			StartRow:      0,
		})
	assert.NoError(self.T(), err)

	golden.Set("Directory file/a/b reading 1000 rows from 0", table)

	// Check the paging works
	table, err = api_service.VFSListDirectoryFiles(self.Ctx,
		&api_proto.GetTableRequest{
			ClientId:      stat.ClientId,
			FlowId:        stat.FlowId,
			VfsComponents: []string{"file", "a", "b"},
			Rows:          1,
			StartRow:      0,
		})
	assert.NoError(self.T(), err)

	golden.Set("Directory file/a/b reading 1 row from 0", table)

	table, err = api_service.VFSListDirectoryFiles(self.Ctx,
		&api_proto.GetTableRequest{
			ClientId:      stat.ClientId,
			FlowId:        stat.FlowId,
			VfsComponents: []string{"file", "a", "b"},
			Rows:          1,
			StartRow:      1,
		})
	assert.NoError(self.T(), err)

	golden.Set("Directory file/a/b reading 1 row from 1", table)

	table, err = api_service.VFSListDirectoryFiles(self.Ctx,
		&api_proto.GetTableRequest{
			ClientId:      stat.ClientId,
			FlowId:        stat.FlowId,
			VfsComponents: []string{"file", "a", "b"},
			Rows:          1000,
			StartRow:      5,
		})
	assert.NoError(self.T(), err)

	golden.Set("Directory file/a/b reading 100 rows from 5", table)

	// Check that deeper directories page properly
	table, err = api_service.VFSListDirectoryFiles(self.Ctx,
		&api_proto.GetTableRequest{
			ClientId:      stat.ClientId,
			FlowId:        stat.FlowId,
			VfsComponents: []string{"file", "a", "b", "c"},
			Rows:          1000,
			StartRow:      5,
		})
	assert.NoError(self.T(), err)

	golden.Set("Directory file/a/b/c reading 100 rows from 0", table)

	table, err = api_service.VFSListDirectoryFiles(self.Ctx,
		&api_proto.GetTableRequest{
			ClientId:      stat.ClientId,
			FlowId:        stat.FlowId,
			VfsComponents: []string{"file", "a", "b", "c"},
			Rows:          1,
			StartRow:      2,
		})
	assert.NoError(self.T(), err)

	golden.Set("Directory file/a/b/c reading 1 rows from 2", table)

	table, err = api_service.VFSListDirectoryFiles(self.Ctx,
		&api_proto.GetTableRequest{
			ClientId:      stat.ClientId,
			FlowId:        stat.FlowId,
			VfsComponents: []string{"file", "a", "b", "c"},
			Rows:          100,
			StartRow:      4,
		})
	assert.NoError(self.T(), err)

	golden.Set("Directory file/a/b/c reading 100 rows from 4", table)

	goldie.Assert(self.T(), "TestRecursiveVFSListDirectoryApiAccess",
		json.MustMarshalIndent(golden))
}

func (self *VFSServiceTestSuite) TestVFSDownload() {
	flow_path_manager := paths.NewFlowPathManager(self.client_id, self.flow_id)

	self.EmulateCollection(
		"System.VFS.ListDirectory", []*ordereddict.Dict{
			makeStat("/a/b", "A"),
			makeStat("/a/b", "B"),
		})

	// Simulate an upload that was received by our
	// System.VFS.DownloadFile collection.
	file_store := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	fd, err := file_store.WriteFile(flow_path_manager.GetUploadsFile(
		"file", "/a/b/B", []string{"a", "b", "B"}).Path())
	assert.NoError(self.T(), err)
	fd.Write([]byte("Data"))
	fd.Close()

	self.EmulateCollection(
		"System.VFS.DownloadFile", []*ordereddict.Dict{
			ordereddict.NewDict().
				Set("Path", "/a/b/B").
				Set("_Components", []string{"a", "b", "B"}).
				Set("Accessor", "file").
				Set("Size", 10),
		})

	vtesting.WaitUntil(2*time.Second, self.T(), func() bool {
		value, pres := file_store.Data.GetString(
			"/clients/C.12312/vfs_files/file/a/b.json")
		return pres &&
			`{"name":"B","size":10,"mtime":1000000000,"components":["clients","C.12312","collections","F.1232","uploads","file","a","b","B"],"flow_id":"F.1232"}
` == value
	})
}

// Create a record for a file
func makeStat(dirname, name string) *ordereddict.Dict {
	fullpath := path.Join(dirname, name)
	return ordereddict.NewDict().
		Set("_FullPath", fullpath).
		Set("Name", name).
		Set("Mode", "-rwx-----").
		Set("_Accessor", "file")
}

// Create a record for a directory
func makeDirectoryStat(dirname, name string) *ordereddict.Dict {
	fullpath := path.Join(dirname, name)
	return ordereddict.NewDict().
		Set("_FullPath", fullpath).
		Set("Name", name).
		Set("Mode", "drwx-----").
		Set("_Accessor", "file")
}

func TestVFSService(t *testing.T) {
	suite.Run(t, &VFSServiceTestSuite{})
}
