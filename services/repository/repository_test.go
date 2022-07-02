package repository_test

import (
	"context"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	defer cancel()

	config_obj.Frontend.ServerServices = &config_proto.ServerServicesConfig{
		JournalService:    true,
		RepositoryManager: true,
	}

	err = orgs.StartTestOrgManager(ctx, wg, config_obj, nil)

	manager, err := services.GetRepositoryManager(config_obj)
	assert.NoError(t, err)

	repository, err := manager.GetGlobalRepository(config_obj)
	assert.NoError(t, err)

	artifact, pres := repository.Get(config_obj, "Custom.TestArtifact")
	assert.True(t, pres)

	assert.Equal(t, artifact.Name, "Custom.TestArtifact")
}
