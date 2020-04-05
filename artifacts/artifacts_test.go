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
	"testing"

	"github.com/stretchr/testify/assert"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/config"
)

var (
	// Artifacts 1 and 2 are invalid: cyclic
	artifact1 = `
name: Artifact1
sources:
  - queries:
      - SELECT * FROM Artifact.Artifact2()
`

	artifact2 = `
name: Artifact2
sources:
  - queries:
      - SELECT * FROM Artifact.Artifact1()
`

	// Artifact3 has no dependencies itself.
	artifact3 = `
name: Artifact3
sources:
  - queries:
      - SELECT * FROM scope()
`

	// Artifact4 depends on 3
	artifact4 = `
name: Artifact4
sources:
  - queries:
      - SELECT * FROM Artifact.Artifact3()
`

	// Artifact5 depends on an unknown artifact
	artifact5 = `
name: Artifact5
sources:
  - queries:
      - SELECT * FROM Artifact.Unknown()
`
)

func TestArtifacts(t *testing.T) {
	repository := NewRepository()
	repository.LoadYaml(artifact1, false)
	repository.LoadYaml(artifact2, false)
	repository.LoadYaml(artifact3, false)
	repository.LoadYaml(artifact4, false)
	repository.LoadYaml(artifact5, false)

	assert := assert.New(t)

	var request *actions_proto.VQLCollectorArgs
	var err error

	// Cycle: Artifact1 -> Artifact2 -> Artifact1
	request = &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				VQL: "SELECT * FROM Artifact.Artifact1()",
			},
		},
	}
	err = repository.PopulateArtifactsVQLCollectorArgs(request)
	assert.Error(err)
	assert.Contains(err.Error(), "Cycle")

	// No Cycle - this should work
	request = &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				VQL: "SELECT * FROM Artifact.Artifact3()",
			},
		},
	}

	err = repository.PopulateArtifactsVQLCollectorArgs(request)
	assert.NoError(err)
	assert.Len(request.Artifacts, 1)
	assert.Equal(request.Artifacts[0].Name, "Artifact3")

	// No Cycle - this should work
	request = &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				VQL: "SELECT * FROM Artifact.Artifact4()",
			},
		},
	}
	err = repository.PopulateArtifactsVQLCollectorArgs(request)
	assert.NoError(err)
	assert.Len(request.Artifacts, 2)

	// Broken - depends on an unknown artifact
	request = &actions_proto.VQLCollectorArgs{
		Query: []*actions_proto.VQLRequest{
			&actions_proto.VQLRequest{
				VQL: "SELECT * FROM Artifact.Artifact5()",
			},
		},
	}
	err = repository.PopulateArtifactsVQLCollectorArgs(request)
	assert.Error(err)
	assert.Contains(err.Error(), "Unknown artifact reference")
}

// Load all built in artifacts and make sure they validate syntax.
func TestArtifactsSyntax(t *testing.T) {
	config_obj := config.GetDefaultConfig()
	repository, err := GetGlobalRepository(config_obj)
	assert.NoError(t, err)

	new_repository := NewRepository()

	for _, artifact_name := range repository.List() {
		artifact, pres := repository.Get(artifact_name)
		assert.True(t, pres)

		_, err = new_repository.LoadProto(artifact, true /* validate */)
		assert.NoError(t, err, "Error compiling "+artifact_name)
	}
}
