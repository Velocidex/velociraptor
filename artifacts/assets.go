/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package artifacts

import (
	"strings"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

func register(config_obj *api_proto.Config) error {
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
				continue
			}
			artifact, err := global_repository.LoadYaml(string(data))
			if err != nil {
				logger.Info("Cant parse asset %s: %s", file, err)
				continue
			}

			artifact.Path = strings.TrimPrefix(
				file, "artifacts/definitions")
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
