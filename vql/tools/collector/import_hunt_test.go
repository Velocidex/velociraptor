package collector_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	file_store_accessor "www.velocidex.com/golang/velociraptor/accessors/file_store"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	hunt_dispatcher_service "www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/server/clients"
	"www.velocidex.com/golang/velociraptor/vql/server/downloads"
	"www.velocidex.com/golang/velociraptor/vql/server/hunts"
	"www.velocidex.com/golang/velociraptor/vql/tools/collector"
	"www.velocidex.com/golang/velociraptor/vtesting"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/data"
)

const (
	fixtureClientId = "C.a99faf363b5601fe"
	fixtureHostname = "devlp"
)

var (
	importHuntArtifacts = []string{`
name: Server.Audit.Logs
type: SERVER_EVENT
`, `
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

func (self *TestSuite) TestCreateAndImportHunt() {
	defer utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))()
	defer utils.SetFlowIdForTests("F.1234")()

	hunt_dispatcher_service.SetHuntIdForTests("H.1234")

	fs_factory := file_store_accessor.NewFileStoreFileSystemAccessor(self.ConfigObj)
	accessors.Register(fs_factory)

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

	acl_manager := acl_managers.NullACLManager{}
	hunt_dispatcher, err := services.GetHuntDispatcher(self.ConfigObj)
	assert.NoError(self.T(), err)

	scope := self.makeScope()
	defer scope.Close()

	ctx := self.Ctx

	hunt, err := hunt_dispatcher.CreateHunt(
		self.Ctx, self.ConfigObj, acl_manager, request)

	assert.NoError(self.T(), err)

	launcher, err := services.GetLauncher(self.ConfigObj)
	assert.NoError(self.T(), err)

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx, self.ConfigObj,
		acl_manager, repository, &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact", "AnotherTestArtifact"},
			Creator:   utils.GetSuperuserName(self.ConfigObj),
			ClientId:  "server",
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

	flow_update := (&hunts.AddToHuntFunction{}).Call(
		ctx, scope, ordereddict.NewDict().
			Set("hunt_id", hunt.HuntId).
			Set("client_id", "server").
			Set("flow_id", flow_id))
	assert.NotEmpty(self.T(), flow_update)

	// Wait here until the collection is completed.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		hunt, pres := hunt_dispatcher.GetHunt(self.Ctx, hunt.HuntId)
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
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		snapshot, _ := self.snapshotHuntFlow().Get("/hunts/H.1234.json")
		return len(json.AnyToString(snapshot, json.DefaultEncOpts())) > 10
	})

	golden := ordereddict.NewDict().
		Set("Original Flow", self.snapshotHuntFlow())

	// Now delete the old hunt
	for _ = range (&hunts.DeleteHuntPlugin{}).Call(ctx, scope,
		ordereddict.NewDict().Set("hunt_id", hunt.HuntId).
			Set("really_do_it", true)) {
	}

	golden.Set("Deleted Flow", self.snapshotHuntFlow())

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()

	imported_hunt := (&collector.ImportCollectionFunction{}).Call(ctx, scope, ordereddict.NewDict().
		Set("filename", download_pathspec).
		Set("accessor", "fs").
		Set("import_type", "hunt"))
	assert.IsType(self.T(), &api_proto.Hunt{}, imported_hunt)

	// Wait here until the hunt is updated - this happens
	// asyncronously by the hunt dispatcher.
	vtesting.WaitUntil(time.Second, self.T(), func() bool {
		snapshot, _ := self.snapshotHuntFlow().Get("/hunts/H.1234.json")
		return len(json.AnyToString(snapshot, json.DefaultEncOpts())) > 10
	})

	// test_utils.GetMemoryFileStore(self.T(), self.ConfigObj).Debug()
	golden.Set("Imported Flow", self.snapshotHuntFlow())

	goldie.Assert(self.T(), "TestCreateAndImportHunt", json.MustMarshalIndent(golden))
}

func (self *TestSuite) snapshotHuntFlow() *ordereddict.Dict {
	return self.snapshot([]string{
		"/clients/server/artifacts/AnotherTestArtifact/F.1234.json",
		"/clients/server/artifacts/AnotherTestArtifact/F.1234.json.index",
		"/clients/server/artifacts/TestArtifact/F.1234.json",
		"/clients/server/artifacts/TestArtifact/F.1234.json.index",
		"/clients/server/collections/F.1234/logs.json",
		"/clients/server/collections/F.1234/logs.json.index",
		"/clients/server/collections/F.1234/uploads/data/Hello",

		"/hunts/H.1234.json",
		"/hunts/H.1234.json.index",
	})
}

func (self *TestSuite) _TestImportHuntFromFixture() {
	self.CreateFlow("server", "F.1234")

	defer utils.SetFlowIdForTests("F.1234XX")()
	defer utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))()

	manager, _ := services.GetRepositoryManager(self.ConfigObj)
	repository, _ := manager.GetGlobalRepository(self.ConfigObj)
	_, err := repository.LoadYaml(CustomTestArtifactDependent,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: true})

	assert.NoError(self.T(), err)

	scope := self.makeScope()
	defer scope.Close()

	ctx := self.Ctx

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
		value, _ := fs.Get("/hunts/H.CKRG32QRAB5N0.json")
		return len(value) > 0
	})
}

func (self *TestSuite) TestImportHuntFromFixture() {
	// Make sure the fixture client id does not exist - importing will
	// create it with the same id.
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = client_info_manager.DeleteClient(
		self.Ctx, fixtureClientId,
		utils.GetSuperuserName(self.ConfigObj),
		services.DiscardDeleteProgress, services.ReallyDoIt)
	assert.NoError(self.T(), err)

	self._TestImportHuntFromFixture()

	goldie.Assert(self.T(), "TestImportHuntFromFixture",
		json.MustMarshalIndent(self.snapshotStaticHuntFlow(fixtureClientId)))
}

func (self *TestSuite) TestImportHuntFromFixtureWithExistingHostname() {
	// Delete the old client id and create a new client with the same
	// hostname.
	client_info_manager, err := services.GetClientInfoManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	err = client_info_manager.DeleteClient(
		self.Ctx, fixtureClientId,
		utils.GetSuperuserName(self.ConfigObj),
		services.DiscardDeleteProgress, services.ReallyDoIt)

	assert.NoError(self.T(), err)

	// Make a new client with a different client id but same
	// hostname. The importer will select this one in favor of the
	// original client id.
	new_client_id := "C.1234abcd"

	scope := self.makeScope()
	defer scope.Close()

	res := clients.NewClientFunction{}.Call(self.Ctx, scope, ordereddict.NewDict().
		Set("client_id", new_client_id).
		Set("hostname", fixtureHostname))
	assert.False(self.T(), utils.IsNil(res))

	self._TestImportHuntFromFixture()

	goldie.Assert(self.T(), "TestImportHuntFromFixtureWithExistingHostname",
		json.MustMarshalIndent(self.snapshotStaticHuntFlow(new_client_id)))
}

func (self *TestSuite) snapshotStaticHuntFlow(
	client_id string) *ordereddict.Dict {
	return self.snapshot([]string{
		"/clients/" + client_id + "/artifacts/Windows.Search.FileFinder/F.CKRG32QRAB5N0.H.json",
		"/clients/" + client_id + "/artifacts/Windows.Search.FileFinder/F.CKRG32QRAB5N0.H.json.index",
		"/clients/" + client_id + "/collections/F.CKRG32QRAB5N0.H/uploads.json",
		"/clients/" + client_id + "/collections/F.CKRG32QRAB5N0.H/uploads.json.index",
		"/clients/" + client_id + "/collections/F.CKRG32QRAB5N0.H/logs.json",
		"/clients/" + client_id + "/collections/F.CKRG32QRAB5N0.H/logs.json.index",

		"/hunts/H.CKRG32QRAB5N0.json",
		"/hunts/H.CKRG32QRAB5N0.json.index",
		"/hunts/H.CKRG32QRAB5N0_errors.json",
		"/hunts/H.CKRG32QRAB5N0_errors.json.index",
	})
}

func (self *TestSuite) snapshot(paths []string) *ordereddict.Dict {
	fs := test_utils.GetMemoryFileStore(self.T(), self.ConfigObj)
	result := ordereddict.NewDict()

	// These are the files we care about in the hunt collection.
	for _, path := range paths {
		value, _ := fs.Get(path)
		golden := string(value)
		if strings.HasSuffix(path, "index") {
			// The index is in binary and depends on the row order.
			result.Set(path, fmt.Sprintf("Index %v bytes", len(value)))
			continue

		} else if strings.HasSuffix(path, "json") {

			// Log files sometimes come out of order so we sort for
			// comparison.
			golden_list := strings.Split(golden, "\n")
			sort.Strings(golden_list)
			result.Set(path, golden_list)
			continue
		}
		result.Set(path, golden)
	}
	return result
}

func (self *TestSuite) makeScope() vfilter.Scope {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	// Now create a download of this collection.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	return manager.BuildScope(builder)
}
