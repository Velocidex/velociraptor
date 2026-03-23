package repository

import (
	"archive/zip"
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

func loadRepositoryFromZipFile(
	ctx context.Context, config_obj *config_proto.Config,
	global_repository services.Repository,
	filename string, options services.ArtifactOptions) (services.Repository, error) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	basename := filepath.Base(filename)

	reader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, fmt.Errorf(
			"artifact definition location %v should be a directory or a zip file: %v",
			filename, err)
	}
	defer reader.Close()

	for _, f := range reader.File {
		if !strings.HasSuffix(f.Name, ".yml") &&
			!strings.HasSuffix(f.Name, ".yaml") {
			continue
		}

		infd, err := reader.Open(f.Name)
		if err != nil {
			continue
		}

		select {
		case <-ctx.Done():
			return nil, errors.New("Cancelled")

		default:
		}

		data, err := utils.ReadAllWithLimit(infd, constants.MAX_MEMORY)
		if err != nil {
			continue
		}

		artifact_obj, err := global_repository.LoadYaml(string(data), options)
		if err != nil {
			logger.Info("Unable to load custom "+
				"artifact %s:%s: %v", basename, f.Name, err)
			continue
		}
		artifact_obj.Raw = string(data)
		logger.Info("Loaded %s:%s", basename, f.Name)
	}

	return global_repository, nil
}

func loadRepositoryFromDirectory(
	ctx context.Context, config_obj *config_proto.Config,
	global_repository services.Repository,
	directory string, options services.ArtifactOptions) (services.Repository, error) {

	stat, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}

	if !stat.IsDir() {
		return loadRepositoryFromZipFile(ctx, config_obj,
			global_repository, directory, options)
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	err = filepath.Walk(directory,
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
