package repository

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/grouper"
	"www.velocidex.com/golang/velociraptor/vql/materializer"
	"www.velocidex.com/golang/velociraptor/vql/remapping"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/explain"
)

func _build(self services.ScopeBuilder, from_scratch bool) vfilter.Scope {
	env := ordereddict.NewDict()
	if self.Env != nil {
		env.MergeFrom(self.Env)
	}
	_, pres := env.Get("_SessionId")
	if !pres {
		env.Set("_SessionId", "")
	}

	// If the repository is not specified, use the global repository.
	if self.Repository == nil {
		manager, err := services.GetRepositoryManager(self.Config)
		if manager == nil || err != nil {
			return vfilter.NewScope()
		}
		self.Repository, err = manager.GetGlobalRepository(self.Config)
		if err != nil {
			return vfilter.NewScope()
		}
	}

	var scope vfilter.Scope
	if from_scratch || self.Config != nil && self.Config.Remappings != nil {
		scope = vql_subsystem.MakeNewScope()
	} else {
		scope = vql_subsystem.MakeScope()
	}

	scope.SetLogger(self.Logger)

	// Make a new fresh cache context.
	scope.SetContext(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	device_manager := accessors.GetDefaultDeviceManager(
		self.Config).Copy()
	env.Set(constants.SCOPE_DEVICE_MANAGER, device_manager)

	if self.Config != nil {
		// Server config contains secrets - they are stored in
		// a way that VQL can not directly access them but
		// plugins can get via vql_subsystem.GetServerConfig()
		vql_subsystem.CacheSet(scope, constants.SCOPE_SERVER_CONFIG, self.Config)

		if self.Config.Client != nil {
			env.Set(constants.SCOPE_CONFIG, self.Config.Client)
		} else {
			// Running on the server, use our own version here.
			env.Set(constants.SCOPE_CONFIG, &config_proto.ClientConfig{
				ServerVersion: config.GetVersion(),
			})
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
	scope.SetGrouper(grouper.NewMergeSortGrouperFactory(self.Config, 10000))
	scope.SetMaterializer(materializer.NewMaterializer())

	// For now explain messages will go to the log stream.
	scope.SetExplainer(explain.NewLoggingExplainer(scope))

	artifact_plugin := NewArtifactRepositoryPlugin(self.Repository, self.Config)
	// Pass the repository into the scope env. Plugins that need to
	// consult the repository should always get it from this variable
	// so it can be subsistuted with an isolate repository for
	// clients.
	env.Set("Artifact", artifact_plugin).
		Set(constants.SCOPE_REPOSITORY, self.Repository)

	scope.AppendVars(env).AddProtocolImpl(
		_ArtifactRepositoryPluginAssociativeProtocol{})

	env.Set(constants.SCOPE_ROOT, scope)

	// If there are remappings in the config file, we apply them to
	// all scopes.
	if self.Config != nil && self.Config.Remappings != nil {
		// We create a pristine copy of the scope so it can be
		// captured in the context of accessors that will be remapped.
		pristine_scope := scope.Copy()
		pristine_scope.AppendVars(ordereddict.NewDict().
			Set(constants.SCOPE_DEVICE_MANAGER,
				accessors.GetDefaultDeviceManager(self.Config).Copy()))

		device_manager.Clear()

		// Pass pristine scope to delegates.
		err := remapping.ApplyRemappingOnScope(
			context.Background(),
			self.Config,
			pristine_scope, // Pristine scope
			scope,          // Remapped scope
			device_manager,
			env, self.Config.Remappings)
		if err != nil {
			scope.Log("Applying remapping: %v", err)
		}

		// Reduce permissions based on the configuration.
		if self.ACLManager != nil {
			new_acl_manager, err := acl_managers.GetRemappingACLManager(
				self.Config, self.ACLManager, self.Config.Remappings)
			if err != nil {
				scope.Log("Applying remapping: %v", err)
			}

			env.Set(vql_subsystem.ACL_MANAGER_VAR, new_acl_manager)
		}
	}

	_ = scope.AddDestructor(func() {
		scope.Log("DEBUG:Query Stats: %v", json.MustMarshalString(
			scope.GetStats().Snapshot()))
	})

	return scope
}

func (self *RepositoryManager) BuildScope(builder services.ScopeBuilder) vfilter.Scope {
	return _build(builder, false)
}

func (self *RepositoryManager) BuildScopeFromScratch(
	builder services.ScopeBuilder) vfilter.Scope {
	return _build(builder, true)
}
