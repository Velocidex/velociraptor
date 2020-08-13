package repository

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
)

type RepositoryManager struct {
	mu         sync.Mutex
	config_obj *config_proto.Config
}

func (self *RepositoryManager) SetArtifactFile(data, required_prefix string) (
	*artifacts_proto.Artifact, error) {

	// First ensure that the artifact is correct.
	tmp_repository := artifacts.NewRepository()
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
	global_repository, err := artifacts.GetGlobalRepository(self.config_obj)
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

	artifact_path_manager := result_sets.NewArtifactPathManager(
		self.config_obj, "", "", "Server.Internal.ArtifactModification")
	services.GetJournal().PushRows(artifact_path_manager, []*ordereddict.Dict{
		ordereddict.NewDict().Set("artifact", artifact.Name).
			Set("op", "set"),
	})

	return artifact, nil
}

func (self *RepositoryManager) DeleteArtifactFile(name string) error {
	global_repository, err := artifacts.GetGlobalRepository(self.config_obj)
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
