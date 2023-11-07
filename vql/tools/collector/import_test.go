package collector_test

import (
	"path/filepath"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/accessors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/flows/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/server/downloads"
	"www.velocidex.com/golang/velociraptor/vql/tools/collector"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
)

func (self *TestSuite) TestImportDynamicCollection() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer closer()

	fs_factory := file_store_accessor.NewFileStoreFileSystemAccessor(self.ConfigObj)
	accessors.Register("fs", fs_factory, "")

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	acl_manager := acl_managers.NullACLManager{}

	// Now create a download of this collection.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)
	client_id := "server"

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx, self.ConfigObj,
		acl_manager, repository, &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact", "AnotherTestArtifact"},
			ClientId:  client_id,
		}, utils.SyncCompleter)
	assert.NoError(self.T(), err)

	// Wait here until the collection is completed.
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		flow, err := launcher.GetFlowDetails(self.Ctx, self.ConfigObj, "server", flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.State == flows_proto.ArtifactCollectorContext_FINISHED
	})

	// Now create the download export. The plugin returns a filestore
	// pathspec to the created download file.
	result := (&downloads.CreateFlowDownload{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", client_id).
			Set("flow_id", flow_id).
			Set("wait", true))

	download_pathspec, ok := result.(path_specs.FSPathSpec)
	assert.True(self.T(), ok)
	assert.NotEmpty(self.T(), download_pathspec.String())

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	golden := ordereddict.NewDict().
		Set("Original Flow", self.snapshotHuntFlow())

	// Now delete the old flow
	for _ = range (&flowseleteFlowPlugin{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", client_id).
			Set("flow_id", flow_id).
			Set("really_do_it", true)) {
	}

	golden.Set("Deleted Flow", self.snapshotHuntFlow())

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	imported_flow := (&collector.ImportCollectionFunction{}).Call(
		ctx, scope, ordereddict.NewDict().
			Set("client_id", client_id).
			Set("filename", download_pathspec).
			Set("accessor", "fs"))
	assert.IsType(self.T(), &flows_proto.ArtifactCollectorContext{}, imported_flow)

	//	test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
	golden.Set("Imported Flow", self.snapshotHuntFlow())

	goldie.Assert(self.T(), "TestImportDynamicCollection", json.MustMarshalIndent(golden))
}

func (self *TestSuite) TestImportStaticCollection() {
	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)
	_, err := repository.LoadYaml(CustomTestArtifactDependent,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)

	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)

	import_file_path, err := filepath.Abs("fixtures/import.zip")
	assert.NoError(self.T(), err)

	result := collector.ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", "auto").
			Set("hostname", "MyNewHost").
			Set("filename", import_file_path))
	context, ok := result.(*proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	// Check the import was successful.
	assert.Equal(self.T(), []string{"Linux.Search.FileFinder"},
		context.ArtifactsWithResults)
	assert.Equal(self.T(), uint64(1), context.TotalCollectedRows)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED,
		context.State)

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Check the indexes are correct for the new client_id
	search_resp, err := indexer.SearchClients(ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{Query: "host:MyNewHost"}, "")
	assert.NoError(self.T(), err)

	// There is one hit - a new client is added to the index.
	assert.Equal(self.T(), 1, len(search_resp.Items))
	assert.Equal(self.T(), search_resp.Items[0].ClientId, context.ClientId)

	// Importing the collection again and providing the same host name
	// will reuse the client id

	result2 := collector.ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", "auto").
			Set("hostname", "MyNewHost").
			Set("filename", import_file_path))
	context2, ok := result2.(*proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	// The new flow was created on the same client id as before.
	assert.Equal(self.T(), context2.ClientId, context.ClientId)
}
