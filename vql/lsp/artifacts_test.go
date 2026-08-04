/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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

package lsp

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	mock_proto "www.velocidex.com/golang/velociraptor/api/mock"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/vfilter"
)

// TestBuildRegistryFromAPIClient checks that artifacts fetched from the
// API client are registered as Artifact.* pseudo-plugins with their
// parameters.
func TestBuildRegistryFromAPIClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mock_proto.NewMockAPIClient(ctrl)
	client.EXPECT().GetArtifacts(gomock.Any(), gomock.Any()).
		Return(&artifacts_proto.ArtifactDescriptors{
			Items: []*artifacts_proto.Artifact{
				{
					Name:        "Windows.Sys.Users",
					Description: "Enumerate users",
					Parameters: []*artifacts_proto.ArtifactParameter{
						{Name: "remoteRegKey", Type: "string"},
					},
				},
				{
					Name:        "Generic.Client.VQL",
					Description: "Run VQL",
					Parameters: []*artifacts_proto.ArtifactParameter{
						{Name: "Command", Type: "string"},
					},
				},
			},
		}, nil)

	scope := vfilter.NewScope()
	defer scope.Close()

	registry, err := BuildRegistryFromAPIClient(
		context.Background(), client, scope)
	require.NoError(t, err)

	// The artifact is registered with the "Artifact." prefix.
	plugin, pres := registry.GetPlugin("Artifact.Windows.Sys.Users")
	require.True(t, pres)
	require.Equal(t, "Enumerate users", plugin.Doc)
	require.Equal(t, "artifact", plugin.Type)
	require.Len(t, plugin.Args, 1)
	require.Equal(t, "remoteRegKey", plugin.Args[0].Name)
	require.Equal(t, "string", plugin.Args[0].Type)

	plugin, pres = registry.GetPlugin("Artifact.Generic.Client.VQL")
	require.True(t, pres)
	require.Equal(t, "Run VQL", plugin.Doc)
	require.Len(t, plugin.Args, 1)
	require.Equal(t, "Command", plugin.Args[0].Name)

	// A completely unknown artifact is not registered.
	_, pres = registry.GetPlugin("Artifact.Bogus.Nope")
	require.False(t, pres)
}
