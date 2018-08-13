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
