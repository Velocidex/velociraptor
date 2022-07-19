package repository

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"os"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

// Loads the global repository with artifacts from the frontend path
// and the file store.
func InitializeGlobalRepositoryFromFilestore(
	ctx context.Context, config_obj *config_proto.Config,
	global_repository services.Repository) (services.Repository, error) {
	if config_obj.Frontend == nil {
		return global_repository, nil
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	// Load artifacts from the custom file store.
	file_store_factory := file_store.GetFileStore(config_obj)
	if file_store_factory == nil {
		return nil, errors.New("Invalid file store")

	}
	err := api.Walk(file_store_factory, paths.ARTIFACT_DEFINITION_PREFIX,
		func(path api.FSPathSpec, info os.FileInfo) error {
			if path.Type() != api.PATH_TYPE_FILESTORE_YAML {
				return nil
			}

			select {
			case <-ctx.Done():
				return errors.New("GetGlobalRepository: Cancelled")

			default:
			}

			// Failing to read a single file is not fatal - just keep
			// going loading the other files.
			fd, err := file_store_factory.ReadFile(path)
			if err != nil {
				logger.Error("GetGlobalRepository: %v", err)
				return nil
			}
			defer fd.Close()

			data, err := ioutil.ReadAll(
				io.LimitReader(fd, constants.MAX_MEMORY))
			if err != nil {
				logger.Error("GetGlobalRepository: %v", err)
				return nil
			}

			artifact_obj, err := global_repository.LoadYaml(
				string(data),
				false, /* validate */
				false /* built_in */)
			if err != nil {
				logger.Info("Unable to load custom "+
					"artifact %s: %v",
					path.AsClientPath(), err)
				return nil
			}
			artifact_obj.Raw = string(data)
			logger.Info("Loaded %s", path.AsClientPath())

			return nil
		})
	if err != nil {
		return nil, err
	}

	return global_repository, nil
}
