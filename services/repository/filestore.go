package repository

import (
	"io"
	"io/ioutil"
	"os"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
)

// Loads the global repository with artifacts from the frontend path
// and the file store.
func InitializeGlobalRepositoryFromFilestore(
	config_obj *config_proto.Config, global_repository *Repository) (*Repository, error) {
	if config_obj.Frontend == nil {
		return global_repository, nil
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	// Load artifacts from the custom file store.
	file_store_factory := file_store.GetFileStore(config_obj)
	err := file_store_factory.Walk(paths.ARTIFACT_DEFINITION_PREFIX,
		func(path api.FSPathSpec, info os.FileInfo) error {
			basename := path.Base()
			if strings.HasSuffix(basename, ".yaml") ||
				strings.HasSuffix(basename, ".yml") {
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
