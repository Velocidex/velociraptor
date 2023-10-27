package collector_test

import (
	"context"
	"path/filepath"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/flows/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/server/downloads"
	"www.velocidex.com/golang/velociraptor/vql/server/hunts"
	"www.velocidex.com/golang/velociraptor/vql/tools/collector"
	"www.velocidex.com/golang/velociraptor/vtesting"

	_ "www.velocidex.com/golang/velociraptor/vql/protocols"
)

func (self *TestSuite) TestImportCollection() {
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

	ctx := context.Background()
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

func (self *TestSuite) TestImportHunt() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository.LoadYaml(`
name: TestArtifact
parameters:
- name: TestArtifact_Arg1
  default: TestArtifact_Arg1_Value

sources:
- query:
    SELECT * FROM info()
`, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: true})

	repository.LoadYaml(`
name: System.Hunt.Creation
type: SERVER_EVENT`, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: true})

	repository.LoadYaml(`
name: AnotherTestArtifact
parameters:
- name: AnotherTestArtifact_Arg1
  default: AnotherTestArtifact_Arg1_Value

sources:
- query:
    SELECT * FROM scope()
`, services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: true})

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

	journal, _ := services.GetJournal(self.ConfigObj)
	assert.NotEmpty(self.T(), journal)

	hunt_id, err := hunt_dispatcher.CreateHunt(
		self.Ctx, self.ConfigObj, acl_manager, request)

	assert.NoError(self.T(), err)

	hunt, pres := hunt_dispatcher.GetHunt(hunt_id)
	assert.True(self.T(), pres, "Hunt should be present.")

	flow_id, err := launcher.ScheduleArtifactCollection(self.Ctx, self.ConfigObj,
		acl_manager, repository, &flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{"TestArtifact", "AnotherTestArtifact"},
			ClientId:  "server",
		}, utils.SyncCompleter)
	assert.NoError(self.T(), err)

	// Wait here until the collection is completed.
	vtesting.WaitUntil(time.Second*50, self.T(), func() bool {
		flow, err := launcher.GetFlowDetails(self.Ctx, self.ConfigObj, "server", flow_id)
		assert.NoError(self.T(), err)

		return flow.Context.State == flows_proto.ArtifactCollectorContext_FINISHED
	})

	flow, err := launcher.GetFlowDetails(self.Ctx, self.ConfigObj, "server", flow_id)
	assert.NoError(self.T(), err)

	assert.ObjectsAreEqual(flows_proto.ArtifactCollectorContext_FINISHED, flow.Context.State)

	flow_update := (&hunts.AddToHuntFunction{}).Call(
		ctx, scope, ordereddict.NewDict().
			Set("hunt_id", hunt_id).
			Set("client_id", "server").
			Set("flow_id", flow_id))
	assert.NotEmpty(self.T(), flow_update)

	// Wait here until the collection is completed.
	vtesting.WaitUntil(time.Second*100, self.T(), func() bool {
		hunt, pres = hunt_dispatcher.GetHunt(hunt_id)
		assert.True(self.T(), pres)

		return hunt.Stats.TotalClientsWithResults >= 1
	})

	hunt.State = api_proto.Hunt_STOPPED
	err = hunt_dispatcher.ModifyHunt(self.Ctx, self.ConfigObj, hunt, hunt.Creator)
	assert.NoError(self.T(), err, "Failed to stop hunt.")

	// Now create the download export. The plugin returns a filestore
	// pathspec to the created download file.
	result := (&downloads.CreateHuntDownload{}).Call(ctx, scope,
		ordereddict.NewDict().
			Set("hunt_id", hunt_id).
			Set("wait", true))

	download_pathspec := result.(path_specs.FSPathSpec)
	assert.NotEmpty(self.T(), download_pathspec)

	imported_hunt := (&collector.ImportCollectionFunction{}).Call(ctx, scope, ordereddict.NewDict().
		Set("file_path", download_pathspec))
	assert.IsType(self.T(), &api_proto.Hunt{}, imported_hunt)
}
