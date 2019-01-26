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
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Install artifact stuff into the scope.
func MakeScope(repository *Repository) *vfilter.Scope {
	scope := vql_subsystem.MakeScope()
	artifact_plugin := NewArtifactRepositoryPlugin(repository, nil)
	env := vfilter.NewDict().Set("Artifact", artifact_plugin)
	return scope.AppendVars(env).AddProtocolImpl(
		_ArtifactRepositoryPluginAssociativeProtocol{})
}
