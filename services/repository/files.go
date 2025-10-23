package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Loads the global repository with artifacts from local filesystem
// directories. You can specify additional artifact directories in
// - Frontend.ArtifactDefinitionsDirectory
// - Defaults.ArtifactDefinitionsDirectories

// Artifacts added through the --definition file will be added to
// these locations.
func InitializeGlobalRepositoryFromFilesystem(
	ctx context.Context, config_obj *config_proto.Config,
	global_repository services.Repository) (services.Repository, error) {
	var err error

	options := services.ArtifactOptions{
		ArtifactIsBuiltIn:    true,
		ArtifactIsCompiledIn: false,
	}

	if config_obj.Frontend != nil &&
		config_obj.Frontend.ArtifactDefinitionsDirectory != "" {
		global_repository, err = loadRepositoryFromDirectory(
			ctx, config_obj, global_repository,
			config_obj.Frontend.ArtifactDefinitionsDirectory, options)
		if err != nil {
			return nil, err
		}
	}

	if config_obj.Defaults != nil {
		for _, directory := range config_obj.Defaults.ArtifactDefinitionsDirectories {
			global_repository, err = loadRepositoryFromDirectory(
				ctx, config_obj, global_repository, directory, options)
			if err != nil {
				return nil, err
			}
		}
	}

	return global_repository, nil
}

func loadRepositoryFromDirectory(
	ctx context.Context, config_obj *config_proto.Config,
	global_repository services.Repository,
	directory string, options services.ArtifactOptions) (services.Repository, error) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	err := filepath.Walk(directory,
		func(path string, finfo os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf(
					"InitializeGlobalRepositoryFromFilesystem: %w", err)
			}

			if finfo.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".yaml") &&
				!strings.HasSuffix(path, ".yml") {
				return nil
			}

			select {
			case <-ctx.Done():
				return errors.New("Cancelled")

			default:
			}

			fd, err := os.Open(path)
			if err != nil {
				return err
			}
			defer fd.Close()

			// Skip files we can not read.
			data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
			if err != nil {
				logger.Error("InitializeGlobalRepositoryFromFilesystem: %v", err)
				return nil
			}

			artifact_obj, err := global_repository.LoadYaml(string(data), options)
			if err != nil {
				logger.Info("Unable to load custom "+
					"artifact %s: %v", path, err)
				return nil
			}
			artifact_obj.Raw = string(data)
			logger.Info("Loaded %s", path)

			return nil
		})

	return global_repository, err
}
