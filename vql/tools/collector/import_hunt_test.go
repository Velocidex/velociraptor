package collector_test

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie"
	"www.velocidex.com/golang/velociraptor/accessors"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/server/downloads"
	"www.velocidex.com/golang/velociraptor/vql/server/hunts"
	"www.velocidex.com/golang/velociraptor/vql/tools/collector"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

var (
	importHuntArtifacts = []string{`
name: TestArtifact
sources:
- query: |
    SELECT "TestArtifact" AS Artifact,
       upload(accessor="data", file="Hello") AS Upload  FROM scope()
`, `
name: System.Hunt.Creation
type: SERVER_EVENT
`, `
name: AnotherTestArtifact
sources:
- query: |
    SELECT "AnotherTestArtifact" AS Artifact FROM scope()
`, `
name: Windows.Search.FileFinder
sources:
- query: SELECT * FROM info()
`,
	}
)

func (self *TestSuite) TestImportDynamicHunt() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer closer()

	fs_factory := file_store_accessor.NewFileStoreFileSystemAccessor(self.ConfigObj)
	accessors.Register("fs", fs_factory, "")

	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	request := &api_proto.Hunt{
		HuntDescription: "My hunt",
		StartRequest: &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact", "AnotherTestArtifact"},
		},
	}

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	acl_manager := acl_managers.NullACLManager{}
	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Now create a download of this collection.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}
	ctx := self.Ctx
	scope := manager.BuildScope(builder)

	hunt, err := hunt_dispatcher.CreateHunt(
		self.Ctx, self.ConfigObj, acl_manager, request)

	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx, self.ConfigObj,
		acl_manager, repository, &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact", "AnotherTestArtifact"},
			ClientId:  "server",
		}, utils.SyncCompleter)
	assert.NoError(self.T(), err)

	// Wait here until the collection is completed.
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		flow, err := launcher.GetFlowDetails(self.Ctx, self.ConfigObj, "server", flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.State == flows_proto.ArtifactCollectorContext_FINISHED
	})

	flow_update := (&hunts.AddToHuntFunction{}).Call(
		ctx, scope, ordereddict.NewDict().
			Set("hunt_id", hunt.HuntId).
			Set("client_id", "server").
			Set("flow_id", flow_id))
	assert.NotEmpty(self.T(), flow_update)

	// Wait here until the collection is completed.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		hunt, pres := hunt_dispatcher.GetHunt(hunt.HuntId)
		assert.True(self.T(), pres)

		return hunt.Stats.TotalClientsWithResults >= 1
	})

	// Now create the download export. The plugin returns a filestore
	// pathspec to the created download file.
	result := (&downloads.CreateHuntDownload{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("hunt_id", hunt.HuntId).
			Set("wait", true))

	download_pathspec, ok := result.(path_specs.FSPathSpec)
	assert.True(self.T(), ok)
	assert.NotEmpty(self.T(), download_pathspec.String())

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	golden := ordereddict.NewDict().
		Set("Original Flow", self.snapshotHuntFlow())

	// Now delete the old hunt
	for row := range (&hunts.DeleteHuntPlugin{}).Call(ctx, scope,
		ordereddict.NewDict().Set("hunt_id", hunt.HuntId).
			Set("really_do_it", true)) {
		json.Dump(row)
	}

	golden.Set("Deleted Flow", self.snapshotHuntFlow())

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	imported_hunt := (&collector.ImportCollectionFunction{}).Call(ctx, scope, ordereddict.NewDict().
		Set("filename", download_pathspec).
		Set("accessor", "fs").
		Set("import_type", "hunt"))
	assert.IsType(self.T(), &api_proto.Hunt{}, imported_hunt)

	json.Dump(imported_hunt)

	//	test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
	golden.Set("Imported Flow", self.snapshotHuntFlow())

	goldie.Assert(self.T(), "TestImportDynamicHunt", json.MustMarshalIndent(golden))
}

func (self *TestSuite) snapshotHuntFlow() *ordereddict.Dict {
	fs := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	result := ordereddict.NewDict()

	// These are the files we care about in the hunt collection.
	for _, path := range []string{
		"/clients/server/artifacts/AnotherTestArtifact/F.1234.json",
		"/clients/server/artifacts/AnotherTestArtifact/F.1234.json.index",
		"/clients/server/artifacts/TestArtifact/F.1234.json",
		"/clients/server/artifacts/TestArtifact/F.1234.json.index",
		"/clients/server/collections/F.1234/logs.json",
		"/clients/server/collections/F.1234/logs.json.index",
		"/clients/server/collections/F.1234/uploads/data/Hello",
	} {
		value, _ := fs.Get(path)
		golden := string(value)
		if strings.HasSuffix(path, "index") {
			golden = hex.EncodeToString(value)
		}
		result.Set(path, golden)
	}
	return result
}

func (self *TestSuite) TestImportStaticHunt() {
	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)
	launcher.SetFlowIdForTests("F.1234XX")

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)
	_, err = repository.LoadYaml(CustomTestArtifactDependent,
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

	import_file_path, err := filepath.Abs("fixtures/import_hunt.zip")
	assert.NoError(self.T(), err)

	result := collector.ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("filename", import_file_path).
			Set("import_type", "hunt"))
	context, ok := result.(*api_proto.Hunt)
	assert.True(self.T(), ok)

	// Check the import was successful.
	assert.Equal(self.T(), []string{"Windows.Search.FileFinder"},
		context.ArtifactSources)
	assert.Equal(self.T(), uint64(1), context.Stats.TotalClientsWithResults)
	assert.Equal(self.T(), api_proto.Hunt_STOPPED, context.State)

	indexer, err := services.GetIndexer(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Check the indexes are correct for the new client_id
	search_resp, err := indexer.SearchClients(ctx, self.ConfigObj,
		&api_proto.SearchClientsRequest{Query: "host:devlp"}, "")
	assert.NoError(self.T(), err)

	// There is one hit - a new client is added to the index.
	assert.Equal(self.T(), 1, len(search_resp.Items))

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	// Wait here until the hunt manager updates the hunt stats
	vtesting.WaitUntil(time.Second*5, self.T(), func() bool {
		fs := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
		value, _ := fs.Get("/hunts/H.CKRG32QRAB5N0_errors.json")
		return len(value) > 0
	})

	goldie.Assert(self.T(), "TestImportStaticHunt",
		json.MustMarshalIndent(self.snapshotStaticHuntFlow()))
}

func (self *TestSuite) snapshotStaticHuntFlow() *ordereddict.Dict {
	fs := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	result := ordereddict.NewDict()

	// These are the files we care about in the hunt collection.
	for _, path := range []string{
		"/clients/C.a99faf363b5601fe/artifacts/Windows.Search.FileFinder/F.CKRG32QRAB5N0.H.json",
		"/clients/C.a99faf363b5601fe/artifacts/Windows.Search.FileFinder/F.CKRG32QRAB5N0.H.json.index",
		"/clients/C.a99faf363b5601fe/collections/F.CKRG32QRAB5N0.H/uploads.json",
		"/clients/C.a99faf363b5601fe/collections/F.CKRG32QRAB5N0.H/uploads.json.index",
		"/clients/C.a99faf363b5601fe/collections/F.CKRG32QRAB5N0.H/logs.json",
		"/clients/C.a99faf363b5601fe/collections/F.CKRG32QRAB5N0.H/logs.json.index",

		"/hunts/H.CKRG32QRAB5N0_errors.json",
		"/hunts/H.CKRG32QRAB5N0_errors.json.index",
	} {
		value, _ := fs.Get(path)
		golden := string(value)
		if strings.HasSuffix(path, "index") {
			golden = hex.EncodeToString(value)
		}
		result.Set(path, golden)
	}
	return result
}
