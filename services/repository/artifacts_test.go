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
package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/config"
)

// Load all built in artifacts and make sure they validate syntax.
func TestArtifactsSyntax(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	manager := &RepositoryManager{config_obj: config_obj}
	repository, err := manager.GetGlobalRepository(config_obj)
	assert.NoError(t, err)

	new_repository := manager.NewRepository()

	for _, artifact_name := range repository.List() {
		artifact, pres := repository.Get(artifact_name)
		assert.True(t, pres)

		_, err = new_repository.LoadProto(artifact, true /* validate */)
		assert.NoError(t, err, "Error compiling "+artifact_name)
	}
}
