package repository

import (
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func _build(wg *sync.WaitGroup, self services.ScopeBuilder, from_scratch bool) *vfilter.Scope {
	env := ordereddict.NewDict()
	if self.Env != nil {
		env.MergeFrom(self.Env)
	}

	if self.Repository == nil {
		manager, _ := services.GetRepositoryManager()
		if manager == nil {
			return vfilter.NewScope()
		}
		self.Repository, _ = manager.GetGlobalRepository(self.Config)
	}

	if self.Config != nil {
		env.Set(constants.SCOPE_SERVER_CONFIG, self.Config)
		if self.Config.Client != nil {
			env.Set(constants.SCOPE_CONFIG, self.Config.Client)
		}
	}
	env.Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	if self.ACLManager != nil {
		env.Set(vql_subsystem.ACL_MANAGER_VAR, self.ACLManager)
	}

	if self.Uploader != nil {
		env.Set(constants.SCOPE_UPLOADER, self.Uploader)
	}

	var scope *vfilter.Scope
	if from_scratch {
		scope = vql_subsystem.MakeNewScope()
	} else {
		scope = vql_subsystem.MakeScope()
	}
	artifact_plugin := NewArtifactRepositoryPlugin(wg, self.Repository.(*Repository))
	env.Set("Artifact", artifact_plugin)

	scope.AppendVars(env).AddProtocolImpl(
		_ArtifactRepositoryPluginAssociativeProtocol{})

	scope.Logger = self.Logger

	env.Set(constants.SCOPE_ROOT, scope)

	return scope
}

func (self *RepositoryManager) BuildScope(builder services.ScopeBuilder) *vfilter.Scope {
	return _build(self.wg, builder, false)
}

func (self *RepositoryManager) BuildScopeFromScratch(
	builder services.ScopeBuilder) *vfilter.Scope {
	return _build(self.wg, builder, true)
}
