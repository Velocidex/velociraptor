package vfs

import (
	"sort"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"

	"www.velocidex.com/golang/velociraptor/accessors/file_store"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	_ "www.velocidex.com/golang/velociraptor/vql/filesystem"
	_ "www.velocidex.com/golang/velociraptor/vql/golang"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers"
)

type testCases struct {
	Path    string
	IsDir   bool
	Content string
}

var (
	Filesystem = []testCases{
		{Path: "C:", IsDir: true},
		{Path: "C:\\Windows", IsDir: true},
		{Path: "C:\\Windows\\System32", IsDir: true},
		{Path: "C:\\Windows\\File1.txt", Content: "File in Windows"},
		{Path: "C:\\Windows\\System32\\File.txt", Content: "File in System32"},
		{Path: "D:", IsDir: true},
	}

	artifacts_used = []string{
		"/artifacts/definitions/System/VFS/ListDirectory.yaml",
		"/artifacts/definitions/System/VFS/DownloadFile.yaml",
	}
)

func setVirtualFilesystem() {
	root_path := accessors.MustNewWindowsOSPath("")
	root_fs_accessor := accessors.NewVirtualFilesystemAccessor(root_path)
	for _, c := range Filesystem {
		root_fs_accessor.SetVirtualFileInfo(&accessors.VirtualFileInfo{
			Path:    accessors.MustNewWindowsOSPath(c.Path),
			IsDir_:  c.IsDir,
			RawData: []byte(c.Content),
			Size_:   int64(len(c.Content)),
		})
	}

	// Install this under a new accessor name.
	accessors.Register(accessors.DescribeAccessor(
		root_fs_accessor, accessors.AccessorDescriptor{
			Name: "vfs_test",
		}))
}

type TestSuite struct {
	test_utils.TestSuite
}

func (self *TestSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.ConfigObj.Services.HuntDispatcher = true
	self.ConfigObj.Services.HuntManager = true
	self.ConfigObj.Services.ServerArtifacts = true
	self.ConfigObj.Services.VfsService = true

	self.TestSuite.SetupTest()
}

func (self *TestSuite) TestVFSAccessor() {
	defer utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))()
	defer utils.SetFlowIdForTests("F.1234")()

	setVirtualFilesystem()

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	options := services.ArtifactOptions{
		ValidateArtifact:     true,
		ArtifactIsBuiltIn:    true,
		ArtifactIsCompiledIn: false,
	}

	for _, a := range artifacts_used {
		data, err := assets.ReadFile(a)
		assert.NoError(self.T(), err)

		_, err = repository.LoadYaml(string(data), options)
		assert.NoError(self.T(), err)
	}

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	var acl_manager vql_subsystem.ACLManager = acl_managers.NullACLManager{}

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx, self.ConfigObj,
		acl_manager, repository, &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"System.VFS.ListDirectory", "System.VFS.DownloadFile"},
			Creator:   utils.GetSuperuserName(self.ConfigObj),
			Specs: []*flows_proto.ArtifactSpec{
				{
					Artifact: "System.VFS.DownloadFile",
					Parameters: &flows_proto.ArtifactParameters{
						Env: []*actions_proto.VQLEnv{
							{
								Key:   "Accessor",
								Value: "vfs_test",
							},
							{
								Key:   "Components",
								Value: "[]",
							},
							{
								Key:   "Recursively",
								Value: "Y",
							},
						},
					},
				},
				{
					Artifact: "System.VFS.ListDirectory",
					Parameters: &flows_proto.ArtifactParameters{
						Env: []*actions_proto.VQLEnv{
							{
								Key:   "Accessor",
								Value: "vfs_test",
							},
							{
								Key:   "Components",
								Value: "[]",
							},
							{
								Key:   "Depth",
								Value: "10",
							},
						},
					}},
			},
			ClientId: "server",
		}, utils.SyncCompleter)
	assert.NoError(self.T(), err)

	// Wait here until the collection is completed.
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		flow, err := launcher.GetFlowDetails(
			self.Ctx, self.ConfigObj, services.GetFlowOptions{},
			"server", flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.State == flows_proto.ArtifactCollectorContext_FINISHED
	})

	vfs_service, err := services.GetVFSService(self.ConfigObj)
	assert.NoError(self.T(), err)

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	// Wait until the vfs service processes it
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		dir, err := vfs_service.ListDirectoryFiles(self.Ctx, self.ConfigObj,
			&api_proto.GetTableRequest{
				Rows:          10,
				ClientId:      "server",
				VfsComponents: []string{"vfs_test", "C:", "Windows"},
			})
		if err != nil {
			return false
		}
		return dir.TotalRows > 0
	})

	fs_factory := file_store.NewFileStoreFileSystemAccessor(self.ConfigObj)
	accessors.Register(accessors.DescribeAccessor(fs_factory,
		accessors.AccessorDescriptor{
			Name: "fs",
		}))

	// Now create a download of this collection.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict().Set("ClientId", "server"),
	}

	scope := manager.BuildScope(builder)

	// Now test the vfs_accessor.
	accessor, err := accessors.GetAccessor("vfs", scope)
	assert.NoError(self.T(), err)

	globber := glob.NewGlobber()
	defer globber.Close()

	glob_path, err := accessors.NewGenericOSPath("/**")
	assert.NoError(self.T(), err)

	globber.Add(glob_path)

	hits := []string{}
	file_content := ordereddict.NewDict()

	for hit := range globber.ExpandWithContext(
		self.Ctx, scope, self.ConfigObj,
		accessors.MustNewGenericOSPath("vfs_test"), accessor) {
		full_path := hit.OSPath().Path()

		hits = append(hits, full_path)

		if !hit.IsDir() {
			data := make([]byte, 1024)
			fd, err := accessor.OpenWithOSPath(hit.OSPath())
			assert.NoError(self.T(), err)

			n, err := fd.Read(data)
			assert.NoError(self.T(), err)
			file_content.Set(full_path, string(data[:n]))
			fd.Close()
		}

	}
	sort.Strings(hits)

	golden := ordereddict.NewDict().
		Set("DirectoryListings", hits).
		Set("FileContents", file_content)

	goldie.Assert(self.T(), "TestVFSAccessor", json.MustMarshalIndent(golden))
}

func TestVFSAccessor(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
