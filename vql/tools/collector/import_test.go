package collector

import (
	"context"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/flows/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"

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

	result := ImportCollectionFunction{}.Call(ctx, scope,
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

	result2 := ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", "auto").
			Set("hostname", "MyNewHost").
			Set("filename", import_file_path))
	context2, ok := result2.(*proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	// The new flow was created on the same client id as before.
	assert.Equal(self.T(), context2.ClientId, context.ClientId)
}
