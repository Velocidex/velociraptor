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

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/vfilter"
)

// BuildRegistryWithArtifacts builds a Registry from a live VQL scope and
// additionally registers all artifacts in the repository as pseudo-plugins.
//
// Artifacts are resolved by the Artifact.* associative protocol at runtime,
// so they do not appear in scope.Describe(). They are the most common
// "plugins" used in real VQL so we add them explicitly.
func BuildRegistryWithArtifacts(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope) (*Registry, error) {

	registry := BuildRegistry(scope)

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return registry, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return registry, err
	}

	names, err := repository.List(ctx, config_obj)
	if err != nil {
		return registry, err
	}

	for _, name := range names {
		artifact, pres := repository.Get(ctx, config_obj, name)
		if !pres || artifact == nil {
			continue
		}

		args := make([]Arg, 0, len(artifact.Parameters))
		for _, param := range artifact.Parameters {
			args = append(args, Arg{
				Name: param.Name,
				Type: param.Type,
			})
		}

		registry.AddArtifact("Artifact."+name, artifact.Description, args)
	}

	return registry, nil
}

// BuildRegistryFromAPIClient builds a Registry from a live VQL scope and
// additionally registers all artifacts fetched from the Velociraptor API.
//
// This is the "hybrid" mode used by the `velociraptor lsp` command: the
// command starts a local API service (like the gui command does) and then
// connects to it as the superuser. Fetching artifacts over the API means
// the LSP sees the same artifacts the API would serve - including custom
// artifacts uploaded to the repository, not just the built-in ones.
func BuildRegistryFromAPIClient(
	ctx context.Context,
	api_client api_proto.APIClient,
	scope vfilter.Scope) (*Registry, error) {

	registry := BuildRegistry(scope)

	// An empty search term matches all artifacts. Fetch a large number
	// since repositories commonly contain more than the 1000 default.
	result, err := api_client.GetArtifacts(ctx, &api_proto.GetArtifactsRequest{
		NumberOfResults: 10000,
	})
	if err != nil {
		return registry, err
	}

	for _, artifact := range result.Items {
		if artifact == nil {
			continue
		}

		args := make([]Arg, 0, len(artifact.Parameters))
		for _, param := range artifact.Parameters {
			args = append(args, Arg{
				Name: param.Name,
				Type: param.Type,
			})
		}

		// VQL references artifacts with the "Artifact." prefix.
		registry.AddArtifact("Artifact."+artifact.Name, artifact.Description, args)
	}

	return registry, nil
}
