package server

import (
	"io"
	"io/ioutil"
	"os"
	"strings"

	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/logging"
)

// Loads the global repository with artifacts from the frontend path
// and the file store.
func GetGlobalRepository(config_obj *config_proto.Config) (*artifacts.Repository, error) {
	global_repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	if config_obj.Frontend.ArtifactsPath != "" {
		count, err := global_repository.LoadDirectory(
			config_obj.Frontend.ArtifactsPath)
		switch errors.Cause(err).(type) {

		// PathError is not fatal - it means we just
		// cant load the directory.
		case *os.PathError:
			logger.Info("Unable to load artifacts from directory "+
				"%s (skipping): %v",
				config_obj.Frontend.ArtifactsPath, err)
		case nil:
			break
		default:
			// Other errors are fatal - they mean we cant
			// parse the artifacts themselves.
			return nil, err
		}
		logger.Info("Loaded %d artifacts from %s",
			*count, config_obj.Frontend.ArtifactsPath)
	}

	// Load artifacts from the custom file store.
	file_store_factory := file_store.GetFileStore(config_obj)
	err = file_store_factory.Walk(constants.ARTIFACT_DEFINITION_PREFIX,
		func(path string, info os.FileInfo, err error) error {
			if err == nil && (strings.HasSuffix(path, ".yaml") ||
				strings.HasSuffix(path, ".yml")) {
				fd, err := file_store_factory.ReadFile(path)
				if err != nil {
					logger.Error(err)
					return nil
				}
				defer fd.Close()

				data, err := ioutil.ReadAll(
					io.LimitReader(fd, constants.MAX_MEMORY))
				if err != nil {
					logger.Error(err)
					return nil
				}

				artifact_obj, err := global_repository.LoadYaml(
					string(data), false /* validate */)
				if err != nil {
					logger.Info("Unable to load custom "+
						"artifact %s: %v", path, err)
					return nil
				}
				artifact_obj.Raw = string(data)
				logger.Info("Loaded %s", path)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}

	return global_repository, nil
}
