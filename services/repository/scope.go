package repository

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
)

func _build(wg *sync.WaitGroup, self services.ScopeBuilder, from_scratch bool) vfilter.Scope {
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

	var scope vfilter.Scope
	if from_scratch {
		scope = vql_subsystem.MakeNewScope()
	} else {
		scope = vql_subsystem.MakeScope()
	}

	scope.SetLogger(self.Logger)

	cache := vql_subsystem.NewScopeCache()
	env.Set(vql_subsystem.CACHE_VAR, cache)

	device_manager := accessors.GlobalDeviceManager.Copy()
	env.Set(constants.SCOPE_DEVICE_MANAGER, device_manager)

	if self.Config != nil {
		// Server config contains secrets - they are stored in
		// a way that VQL can not directly access them but
		// plugins can get via vql_subsystem.GetServerConfig()
		cache.Set(constants.SCOPE_SERVER_CONFIG, self.Config)

		if self.Config.Client != nil {
			env.Set(constants.SCOPE_CONFIG, self.Config.Client)
		} else {
			env.Set(constants.SCOPE_CONFIG, &config_proto.ClientConfig{
				Version: config.GetVersion(),
			})
		}

		// If there are remappings in the config file, we apply them to
		// all scopes.
		if self.Config.Remappings != nil {
			device_manager.Clear()
			err := accessors.ApplyRemappingOnScope(
				context.Background(), device_manager, self.Config.Remappings)
			if err != nil {
				scope.Log("Applying remapping: %v", err)
			}

			self.ACLManager = accessors.GetRemappingACLManager(
				self.Config.Remappings)
		}
	}

	// Builder can contain only the client config if it is running on
	// the client.
	if self.ClientConfig != nil {
		env.Set(constants.SCOPE_CONFIG, self.ClientConfig)
	}

	if self.ACLManager != nil {
		env.Set(vql_subsystem.ACL_MANAGER_VAR, self.ACLManager)
	}

	if self.Uploader != nil {
		env.Set(constants.SCOPE_UPLOADER, self.Uploader)
	}

	// Use our own sorter
	scope.SetSorter(sorter.MergeSorter{ChunkSize: 10000})

	artifact_plugin := NewArtifactRepositoryPlugin(wg, self.Repository.(*Repository))
	env.Set("Artifact", artifact_plugin)

	scope.AppendVars(env).AddProtocolImpl(
		_ArtifactRepositoryPluginAssociativeProtocol{})

	env.Set(constants.SCOPE_ROOT, scope)

	_ = scope.AddDestructor(func() {
		scope.Log("Query Stats: %v", json.MustMarshalString(
			scope.GetStats().Snapshot()))
	})

	return scope
}

func (self *RepositoryManager) BuildScope(builder services.ScopeBuilder) vfilter.Scope {
	return _build(self.wg, builder, false)
}

func (self *RepositoryManager) BuildScopeFromScratch(
	builder services.ScopeBuilder) vfilter.Scope {
	return _build(self.wg, builder, true)
}
