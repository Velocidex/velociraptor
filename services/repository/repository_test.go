package repository_test

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	repository_impl "www.velocidex.com/golang/velociraptor/services/repository"
)

func TestLoadingFromFilestore(t *testing.T) {
	config_obj, err := new(config.Loader).
		WithFileLoader("../../http_comms/test_data/server.config.yaml").
		LoadAndValidate()
	assert.NoError(t, err)

	tmpdir, err := ioutil.TempDir("", "tmp")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	config_obj.Datastore.Implementation = "FileBaseDataStore"
	config_obj.Datastore.Location = tmpdir
	config_obj.Datastore.FilestoreDirectory = tmpdir
	config_obj.Frontend.DoNotCompressArtifacts = true

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile(paths.GetArtifactDefintionPath(
		"Custom.TestArtifact"))
	assert.NoError(t, err)

	fd.Truncate()
	fd.Write([]byte(`name: Custom.TestArtifact`))
	fd.Close()

	ctx, _ := context.WithTimeout(context.Background(), time.Second*60)
	sm := services.NewServiceManager(ctx, config_obj)
	defer sm.Close()

	require.NoError(t, sm.Start(journal.StartJournalService))
	require.NoError(t, sm.Start(repository_impl.StartRepositoryManager))

	manager, err := services.GetRepositoryManager()
	assert.NoError(t, err)

	repository, err := manager.GetGlobalRepository(config_obj)
	assert.NoError(t, err)

	artifact, pres := repository.Get(config_obj, "Custom.TestArtifact")
	assert.True(t, pres)

	assert.Equal(t, artifact.Name, "Custom.TestArtifact")
}
