package artifacts

import (
	"strings"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	config "www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

func register(config_obj *config.Config) error {
	files, err := assets.WalkDirs("", false)
	if err != nil {
		return err
	}

	count := 0
	logger := logging.NewLogger(config_obj)
	for _, file := range files {
		if strings.HasPrefix(file, "artifacts/definitions") &&
			strings.HasSuffix(file, "yaml") {
			data, err := assets.ReadFile(file)
			if err != nil {
				logger.Info("Cant read asset %s: %v", file, err)
				continue
			}
			err = global_repository.LoadYaml(string(data))
			if err != nil {
				logger.Info("Cant parse asset %s: %s", file, err)
				continue
			}
			count += 1
		}
	}

	logger.Info("Loaded %d built in artifacts", count)
	return nil
}

// Load basic artifacts from our assets.
func init() {
	init_registry = append(init_registry, register)
}
