package repository_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func TestLoadingFromFilestore(t *testing.T) {
	config_obj, err := new(config.Loader).
		WithFileLoader("../../http_comms/test_data/server.config.yaml").
		LoadAndValidate()
	assert.NoError(t, err)

	tmpdir, err := tempfile.TempDir("tmp")
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

	config_obj.Services = &config_proto.ServerServicesConfig{
		JournalService:    true,
		RepositoryManager: true,
	}

	err = orgs.StartTestOrgManager(ctx, wg, config_obj, nil)

	manager, err := services.GetRepositoryManager(config_obj)
	assert.NoError(t, err)

	repository, err := manager.GetGlobalRepository(config_obj)
	assert.NoError(t, err)

	artifact, pres := repository.Get(ctx, config_obj, "Custom.TestArtifact")
	assert.True(t, pres)

	assert.Equal(t, artifact.Name, "Custom.TestArtifact")
}

func TestOverrideBuiltInArtifacts(t *testing.T) {
	config_obj, err := new(config.Loader).
		WithFileLoader("../../http_comms/test_data/server.config.yaml").
		LoadAndValidate()
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	defer cancel()

	config_obj.Services = &config_proto.ServerServicesConfig{
		JournalService:    true,
		RepositoryManager: true,
	}

	err = orgs.StartTestOrgManager(ctx, wg, config_obj, nil)

	manager, err := services.GetRepositoryManager(config_obj)
	assert.NoError(t, err)

	repository, err := manager.GetGlobalRepository(config_obj)
	assert.NoError(t, err)

	// Set a built in artifact
	_, err = repository.LoadYaml(`name: Custom.BuiltIn`,
		services.ArtifactOptions{
			ArtifactIsBuiltIn: true,
			ValidateArtifact:  true,
		})
	assert.NoError(t, err)

	artifact, pres := repository.Get(ctx, config_obj, "Custom.BuiltIn")
	assert.True(t, pres)

	// Now try to override it - not a built in should fail
	_, err = repository.LoadYaml(`name: Custom.BuiltIn`,
		services.ArtifactOptions{
			ArtifactIsBuiltIn: false,
			ValidateArtifact:  true,
		})
	assert.Error(t, err)

	// Now try to override it with a built in artifact
	_, err = repository.LoadYaml(`
name: Custom.BuiltIn
description: Override
`, services.ArtifactOptions{
		ArtifactIsBuiltIn: true,
		ValidateArtifact:  true,
	})
	assert.NoError(t, err)

	artifact, pres = repository.Get(ctx, config_obj, "Custom.BuiltIn")
	assert.True(t, pres)

	assert.Equal(t, artifact.Name, "Custom.BuiltIn")
	assert.Equal(t, artifact.Description, "Override")
}

func TestArtifactMetadata(t *testing.T) {
	config_obj, err := new(config.Loader).
		WithFileLoader("../../http_comms/test_data/server.config.yaml").
		LoadAndValidate()
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	defer cancel()

	config_obj.Services = &config_proto.ServerServicesConfig{
		JournalService:    true,
		RepositoryManager: true,
	}

	err = orgs.StartTestOrgManager(ctx, wg, config_obj, nil)
	manager, err := services.GetRepositoryManager(config_obj)
	assert.NoError(t, err)

	repository, err := manager.GetGlobalRepository(config_obj)
	assert.NoError(t, err)

	// Set a built in artifact
	_, err = repository.LoadYaml(`name: Custom.BuiltIn`,
		services.ArtifactOptions{
			ArtifactIsBuiltIn: true,
			ValidateArtifact:  true,
		})
	assert.NoError(t, err)

	artifact, pres := repository.Get(ctx, config_obj, "Custom.BuiltIn")
	assert.True(t, pres)

	// No metadata
	assert.True(t, artifact.Metadata == nil)

	// Set it to be hidden
	err = manager.SetArtifactMetadata(ctx, config_obj, "principal",
		"Custom.BuiltIn", &artifacts_proto.ArtifactMetadata{
			Hidden: true,
		})
	assert.NoError(t, err)

	artifact, pres = repository.Get(ctx, config_obj, "Custom.BuiltIn")
	assert.True(t, pres)
	assert.True(t, artifact.Metadata.Hidden)

	metadata_storage := &artifacts_proto.ArtifactMetadataStorage{}
	path_manager := paths.RepositoryPathManager{}
	db, err := datastore.GetDB(config_obj)
	assert.NoError(t, err)

	err = db.GetSubject(config_obj, path_manager.Metadata(), metadata_storage)
	assert.NoError(t, err)

	goldie.AssertJson(t, "TestArtifactMetadata", metadata_storage)
}
