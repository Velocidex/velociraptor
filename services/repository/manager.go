package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

type RepositoryManager struct {
	mu                sync.Mutex
	config_obj        *config_proto.Config
	global_repository *Repository
}

func (self *RepositoryManager) NewRepository() services.Repository {
	return &Repository{
		config_obj: self.config_obj,
		Data:       make(map[string]*artifacts_proto.Artifact)}
}

func (self *RepositoryManager) GetGlobalRepository(
	config_obj *config_proto.Config) (services.Repository, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.global_repository != nil {
		return self.global_repository, nil
	}

	self.global_repository = &Repository{
		config_obj: self.config_obj,
		Data:       make(map[string]*artifacts_proto.Artifact)}

	now := time.Now()

	assets.Init()
	files, err := assets.WalkDirs("", false)
	if err != nil {
		return nil, err
	}

	count := 0
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	for _, file := range files {
		if strings.HasPrefix(file, "artifacts/definitions") &&
			strings.HasSuffix(file, "yaml") {
			data, err := assets.ReadFile(file)
			if err != nil {
				logger.Info("Cant read asset %s: %v", file, err)
				continue
			}
			_, err = self.global_repository.LoadYaml(
				string(data), false /* Validate */)
			if err != nil {
				logger.Info("Cant parse asset %s: %s", file, err)
				continue
			}

			count += 1
		}
	}

	// Compile the artifacts in the background so they are ready
	// to go when the GUI searches for them.
	go func() {
		for _, name := range self.global_repository.List() {
			self.global_repository.Get(name)
		}
		logger.Info("Compiled all artifacts.")
	}()

	logger.Info("Loaded %d built in artifacts in %v", count, time.Now().Sub(now))
	return self.global_repository, nil
}

func (self *RepositoryManager) SetGlobalRepositoryForTests(repository services.Repository) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.global_repository = repository.(*Repository)
}

func (self *RepositoryManager) SetArtifactFile(data, required_prefix string) (
	*artifacts_proto.Artifact, error) {

	// First ensure that the artifact is correct.
	tmp_repository := self.NewRepository()
	artifact_definition, err := tmp_repository.LoadYaml(
		data, true /* validate */)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(artifact_definition.Name, required_prefix) {
		return nil, errors.New(
			"Modified or custom artifacts must start with '" +
				required_prefix + "'")
	}

	file_store_factory := file_store.GetFileStore(self.config_obj)

	// Load the new artifact into the global repo so it is
	// immediately available.
	global_repository, err := self.GetGlobalRepository(self.config_obj)
	if err != nil {
		return nil, err
	}

	vfs_path := paths.GetArtifactDefintionPath(artifact_definition.Name)

	// Now write it into the filestore.
	fd, err := file_store_factory.WriteFile(vfs_path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	// We want to completely replace the content of the file.
	err = fd.Truncate()
	if err != nil {
		return nil, err
	}

	_, err = fd.Write([]byte(data))
	if err != nil {
		return nil, err
	}

	// Load the artifact into the currently running repository.
	// Artifact is already valid - no need to revalidate it again.
	artifact, err := global_repository.LoadYaml(data, false /* validate */)
	if err != nil {
		return nil, err
	}

	services.GetJournal().PushRowsToArtifact([]*ordereddict.Dict{
		ordereddict.NewDict().Set("artifact", artifact.Name).
			Set("op", "set"),
	}, "Server.Internal.ArtifactModification")

	return artifact, nil
}

func (self *RepositoryManager) DeleteArtifactFile(name string) error {
	global_repository, err := self.GetGlobalRepository(self.config_obj)
	if err != nil {
		return err
	}

	_, pres := global_repository.Get(name)
	if !pres {
		return nil
	}

	file_store_factory := file_store.GetFileStore(self.config_obj)

	global_repository.Del(name)

	vfs_path := paths.GetArtifactDefintionPath(name)
	return file_store_factory.Delete(vfs_path)

}

func StartRepositoryManager(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	services.RegisterRepositoryManager(&RepositoryManager{
		config_obj: config_obj,
	})
	return nil
}
