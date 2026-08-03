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
