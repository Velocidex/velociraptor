package repository

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"google.golang.org/protobuf/proto"
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
	global_repository *Repository
	wg                *sync.WaitGroup
	config_obj        *config_proto.Config
	metadata          *metadataManager
}

func (self *RepositoryManager) NewRepository() services.Repository {
	result := &Repository{
		Data:     make(map[string]*artifacts_proto.Artifact),
		metadata: self.metadata,
	}

	return result
}

// Watch for updates from other nodes to the repository manager.
func (self *RepositoryManager) StartWatchingForUpdates(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	// Are we running on the client? we dont need to sync local
	// repository managers.
	if config_obj.Services != nil &&
		config_obj.Services.ClientEventTable {
		return nil
	}

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

					artifact, err := global_repository.LoadYaml(
						definition, services.ArtifactOptions{
							ValidateArtifact:  false,
							ArtifactIsBuiltIn: false})

					if err == nil {
						logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
						logger.Info("Updating artifact %v in local repository", artifact.Name)
					}

				default:
					logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
					logger.Error("RepositoryManager: <red>Unknown op %v</>", op)
				}
			}
		}
	}()

	return nil
}

func (self *RepositoryManager) SetArtifactMetadata(
	ctx context.Context, config_obj *config_proto.Config,
	principal, name string, metadata *artifacts_proto.ArtifactMetadata) error {

	self.metadata.Set(name, metadata)

	// Tell interested parties that we modified this artifact.
	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	err = journal.PushRowsToArtifact(ctx, config_obj,
		[]*ordereddict.Dict{
			ordereddict.NewDict().
				Set("setter", principal).
				Set("artifact", name).
				Set("op", "metadata").
				Set("metadata", metadata).
				Set("id", self.id),
		}, "Server.Internal.ArtifactModification", "server", "")

	return err
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

	self.global_repository = repository.(*Repository)
}

func (self *RepositoryManager) SetArtifactFile(
	ctx context.Context, config_obj *config_proto.Config,
	principal, definition, required_prefix string) (
	*artifacts_proto.Artifact, error) {

	// Use regexes to force the artifact into the correct prefix.
	if required_prefix != "" {
		definition = ensureArtifactPrefix(definition, required_prefix)
	}

	// Ensure that the artifact is correct by parsing it.
	tmp_repository := self.NewRepository()
	artifact_definition, err := tmp_repository.LoadYaml(
		definition, services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: false})
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
		definition, services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: false})
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

	err = journal.PushRowsToArtifact(ctx, config_obj,
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

func (self *RepositoryManager) SetParent(
	config_obj *config_proto.Config, parent services.Repository) {
	self.global_repository.SetParent(parent, config_obj)
}

func (self *RepositoryManager) DeleteArtifactFile(
	ctx context.Context, config_obj *config_proto.Config,
	principal, name string) error {
	global_repository, err := self.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	// If not there nothing to do...
	_, pres := global_repository.Get(ctx, config_obj, name)
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

	err = journal.PushRowsToArtifact(ctx, config_obj,
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
	self := _newRepositoryManager(ctx, config_obj, wg)

	// Load some artifacts via the autoexec mechanism.
	if config_obj.Autoexec != nil {
		for _, def := range config_obj.Autoexec.ArtifactDefinitions {
			_, err := self.global_repository.LoadProto(
				def, services.ArtifactOptions{
					ValidateArtifact:  true,
					ArtifactIsBuiltIn: true,
				})
			if err != nil {
				return nil, err
			}
		}
	}

	return self, self.StartWatchingForUpdates(ctx, wg, config_obj)
}

func _newRepositoryManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) *RepositoryManager {

	global_repository := &Repository{
		// Artifact name -> definition
		Data:     make(map[string]*artifacts_proto.Artifact),
		metadata: NewMetadataManager(ctx, config_obj),
	}

	// Start the metada house keeping loop.
	wg.Add(1)
	go global_repository.metadata.HouseKeeping(
		ctx, config_obj, wg, global_repository)

	// Shared between the manager repositories.
	return &RepositoryManager{
		wg:                wg,
		id:                utils.GetId(),
		config_obj:        config_obj,
		global_repository: global_repository,
		metadata:          global_repository.metadata,
	}
}

func NewRepositoryManager(ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.RepositoryManager, error) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting repository manager for %v", services.GetOrgName(config_obj))

	// Load all the artifacts in the repository and compile them in the background.
	self := _newRepositoryManager(ctx, config_obj, wg)

	// Backup the custom artifacts - only for the Master node.
	if services.IsMaster(config_obj) && !services.IsClient(config_obj) {
		backup_service, err := services.GetBackupService(config_obj)
		if err == nil {
			backup_service.Register(&RepositoryBackupProvider{
				config_obj: config_obj,
			})
		}
	}

	return self, self.StartWatchingForUpdates(ctx, wg, config_obj)
}

// Loads artifacts that are defined directly in th Autoexec config
// section. These are considered built in (so they can not be
// modified) but are not actually compiled in.
func LoadArtifactsFromConfig(
	repo_manager services.RepositoryManager,
	config_obj *config_proto.Config) error {
	global_repository, err := repo_manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	options := services.ArtifactOptions{
		ValidateArtifact:     true,
		ArtifactIsBuiltIn:    true,
		ArtifactIsCompiledIn: false,
	}

	// Load some artifacts via the autoexec mechanism.
	if config_obj.Autoexec != nil {
		for _, def := range config_obj.Autoexec.ArtifactDefinitions {
			def = proto.Clone(def).(*artifacts_proto.Artifact)

			// These artifacts do not actually have a raw section so
			// create one for them.
			serialize, err := yaml.Marshal(def)
			if err == nil {
				def.Raw = string(serialize)
			}

			// Artifacts loaded from the config file are considered built in.
			_, err = global_repository.LoadProto(def, options)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func LoadBuiltInArtifacts(ctx context.Context,
	config_obj *config_proto.Config,
	self *RepositoryManager) error {

	// Load the built in artifacts as built in. NOTE: Built in
	// artifacts can not be overwritten!
	options := services.ArtifactOptions{
		ValidateArtifact:     false,
		ArtifactIsBuiltIn:    true,
		ArtifactIsCompiledIn: true,
	}

	now := time.Now()

	count := 0
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	for file := range assets.Inventory {
		if strings.HasPrefix(file, "/artifacts/definitions") &&
			strings.HasSuffix(file, "yaml") {
			data, err := assets.ReadFile(file)
			if err != nil {
				logger.Info("Cant read asset %s: %v", file, err)
				if options.ValidateArtifact {
					return err
				}
				continue
			}

			_, err = self.global_repository.LoadYaml(string(data), options)
			if err != nil {
				logger.Info("Cant parse asset %s: %s", file, err)
				if options.ValidateArtifact {
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
				_, pres := grepository.Get(ctx, config_obj, name)
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
