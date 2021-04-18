package tools

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"www.velocidex.com/golang/velociraptor/flows/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/labels"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func (self *TestSuite) TestImportCollection() {
	require.NoError(self.T(), self.sm.Start(labels.StartLabelService))

	manager, _ := services.GetRepositoryManager()
	repository, _ := manager.GetGlobalRepository(self.config_obj)
	_, err := repository.LoadYaml(CustomTestArtifactDependent, true)
	assert.NoError(self.T(), err)

	builder := services.ScopeBuilder{
		Config:     self.config_obj,
		ACLManager: vql_subsystem.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.config_obj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := context.Background()
	scope := manager.BuildScope(builder)

	import_file_path, err := filepath.Abs("fixtures/import.zip")
	assert.NoError(self.T(), err)

	fd, err := os.Open(import_file_path)
	assert.NoError(self.T(), err)
	defer fd.Close()
	data, err := ioutil.ReadAll(fd)
	assert.NoError(self.T(), err)

	result := ImportCollectionFunction{}.Call(ctx, scope,
		ordereddict.NewDict().
			Set("client_id", "auto").
			Set("hostname", "MyNewHost").
			Set("accessor", "data").
			Set("filename", data))
	context, ok := result.(*proto.ArtifactCollectorContext)
	assert.True(self.T(), ok)

	// Check the improt was successful.
	assert.Equal(self.T(), []string{"Custom.TestArtifactDependent"},
		context.ArtifactsWithResults)
	assert.Equal(self.T(), uint64(1), context.TotalCollectedRows)
	assert.Equal(self.T(), flows_proto.ArtifactCollectorContext_FINISHED,
		context.State)
}
