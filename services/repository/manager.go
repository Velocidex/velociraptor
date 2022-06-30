package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
	"www.velocidex.com/golang/velociraptor/utils"
)

type RepositoryManager struct {
	mu                sync.Mutex
	id                uint64
	global_repository services.Repository
	wg                *sync.WaitGroup
}

func (self *RepositoryManager) NewRepository() services.Repository {
	return &Repository{
		Data: make(map[string]*artifacts_proto.Artifact)}
}

// Watch for updates from other nodes to the repository manager.
func (self *RepositoryManager) StartWatchingForUpdates(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	row_chan, cancel := journal.Watch(ctx,
		"Server.Internal.ArtifactModification",
		"RepositoryManager")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return

			case row, ok := <-row_chan:
				if !ok {
					return
				}

				// Only watch for events from other nodes.
				id, pres := row.GetInt64("id")
				if !pres || uint64(id) == self.id {
					continue
				}

				op, _ := row.GetString("op")
				switch op {
				case "delete":
					artifact_name, _ := row.GetString("artifact")

					global_repository, err := self.GetGlobalRepository(
						config_obj)
					if err != nil {
						continue
					}

					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Info("Removing artifact %v from local repository",
						artifact_name)
					global_repository.Del(artifact_name)

				case "set":
					// Refresh the artifact from the filestore.
					definition, pres := row.GetString("definition")
					if !pres {
						continue
					}

					global_repository, err := self.GetGlobalRepository(
						config_obj)
					if err != nil {
						continue
					}

					artifact, err := global_repository.LoadYaml(definition,
						false, /* validate */
						false /* built_in */)

					if err == nil {
						logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
						logger.Info("Updating artifact %v in local repository", artifact.Name)
					}

				default:
					fmt.Printf("Unknown op %v\n", op)
				}
			}
		}
	}()

	return nil
}

func (self *RepositoryManager) GetGlobalRepository(
	config_obj *config_proto.Config) (services.Repository, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.global_repository, nil
}

func (self *RepositoryManager) SetGlobalRepositoryForTests(
	config_obj *config_proto.Config, repository services.Repository) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.global_repository = repository.(services.Repository)
}

func (self *RepositoryManager) SetArtifactFile(
	config_obj *config_proto.Config, principal, definition, required_prefix string) (
	*artifacts_proto.Artifact, error) {

	// Use regexes to force the artifact into the correct prefix.
	if required_prefix != "" {
		definition = ensureArtifactPrefix(definition, required_prefix)
	}

	// Ensure that the artifact is correct by parsing it.
	tmp_repository := self.NewRepository()
	artifact_definition, err := tmp_repository.LoadYaml(
		definition, true /* validate */, false /* built_in */)
	if err != nil {
		return nil, err
	}

	// This should only be triggered if something weird happened.
	if !strings.HasPrefix(artifact_definition.Name, required_prefix) {
		return nil, errors.New(
			"Modified or custom artifacts must start with '" +
				required_prefix + "'")
	}

	// Load the new artifact into the global repo so it is
	// immediately available.
	global_repository, err := self.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	// Load the artifact into the currently running repository.
	artifact, err := global_repository.LoadYaml(
		definition, true /* validate */, false /* built_in */)
	if err != nil {
		return nil, err
	}

	// Artifact should be valid now so we can write it on disk.
	file_store_factory := file_store.GetFileStore(config_obj)
	if file_store_factory != nil {
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

		_, err = fd.Write([]byte(definition))
		if err != nil {
			return nil, err
		}
	}

	// Tell interested parties that we modified this artifact.
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return nil, err
	}

	err = journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("setter", principal).
				Set("artifact", artifact.Name).
				Set("op", "set").
				Set("definition", definition).
				Set("id", self.id),
		}, "Server.Internal.ArtifactModification", "server", "")

	return artifact, err
}

func (self *RepositoryManager) DeleteArtifactFile(
	config_obj *config_proto.Config, principal, name string) error {
	global_repository, err := self.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	// If not there nothing to do...
	_, pres := global_repository.Get(config_obj, name)
	if !pres {
		return nil
	}

	// Remove the artifact from the repository.
	global_repository.Del(name)

	// Now let interested parties know it is removed.
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	err = journal.PushRowsToArtifact(config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("setter", principal).
				Set("artifact", name).
				Set("op", "delete").
				Set("id", self.id),
		}, "Server.Internal.ArtifactModification", "server", "")
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(config_obj)

	// Delete it from the filestore.
	vfs_path := paths.GetArtifactDefintionPath(name)
	return file_store_factory.Delete(vfs_path)
}

// Start a mostly empty repository manager without loading built in
// artifacts.
func NewRepositoryManagerForTest(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.RepositoryManager, error) {
	self := _newRepositoryManager(wg)

	// Load some artifacts via the autoexec mechanism.
	if config_obj.Autoexec != nil {
		for _, def := range config_obj.Autoexec.ArtifactDefinitions {
			_, err := self.global_repository.LoadProto(def, true)
			if err != nil {
				return nil, err
			}
		}
	}

	return self, self.StartWatchingForUpdates(ctx, wg, config_obj)
}

func _newRepositoryManager(wg *sync.WaitGroup) *RepositoryManager {
	return &RepositoryManager{
		wg: wg,
		id: utils.GetId(),
		global_repository: &Repository{
			Data: make(map[string]*artifacts_proto.Artifact),
		},
	}
}

func NewRepositoryManager(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.RepositoryManager, error) {

	// Load all the artifacts in the repository and compile them in the background.
	self := _newRepositoryManager(wg)

	// Assume the built in artifacts are OK so we dont need to
	// validate them at runtime.
	err := LoadBuiltInArtifacts(ctx, config_obj, self, false /* validate */)
	if err != nil {
		return nil, err
	}

	return self, self.StartWatchingForUpdates(ctx, wg, config_obj)
}

func LoadBuiltInArtifacts(ctx context.Context,
	config_obj *config_proto.Config,
	self *RepositoryManager, validate bool) error {

	now := time.Now()

	assets.Init()

	files, err := assets.WalkDirs("", false)
	if err != nil {
		return err
	}

	count := 0
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	for _, file := range files {
		if strings.HasPrefix(file, "artifacts/definitions") &&
			strings.HasSuffix(file, "yaml") {
			data, err := assets.ReadFile(file)
			if err != nil {
				logger.Info("Cant read asset %s: %v", file, err)
				if validate {
					return err
				}
				continue
			}

			// Load the built in artifacts as built in. NOTE: Built in
			// artifacts can not be overwritten!
			_, err = self.global_repository.LoadYaml(
				string(data), validate /* Validate */, true /* built_in */)
			if err != nil {
				logger.Info("Cant parse asset %s: %s", file, err)
				if validate {
					return err
				}
				continue
			}

			count += 1
		}
	}

	grepository, err := InitializeGlobalRepositoryFromFilesystem(
		ctx, config_obj, self.global_repository)
	if err != nil {
		return err
	}

	grepository, err = InitializeGlobalRepositoryFromFilestore(
		ctx, config_obj, self.global_repository)
	if err != nil {
		return err
	}

	// Compile the artifacts in the background so they are ready
	// to go when the GUI searches for them.
	self.wg.Add(1)
	go func() {
		defer self.wg.Done()
		names, err := grepository.List(ctx, config_obj)
		if err != nil {
			logger.Error("Error: %v", err)
			return
		}

		for _, name := range names {
			select {
			case <-ctx.Done():
				return

			default:
				_, pres := grepository.Get(config_obj, name)
				if !pres {
					grepository.Del(name)
				}
			}
		}
		logger.Info("Compiled all artifacts.")
		grepository.Del("")
	}()

	logger.Info("Loaded %d built in artifacts in %v", count, time.Since(now))

	return nil
}

var (
	name_regex = regexp.MustCompile("(?sm)^(name: *)(.+)$")
)

func ensureArtifactPrefix(definition, prefix string) string {
	return utils.ReplaceAllStringSubmatchFunc(
		name_regex, definition,
		func(matches []string) string {
			if !strings.HasPrefix(matches[2], prefix) {
				return matches[1] + prefix + matches[2]
			}
			return matches[1] + matches[2]
		})
}
