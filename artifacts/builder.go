package artifacts

import (
	"log"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Make it easier to build a query scope using the aritfact
// repository.
type ScopeBuilder struct {
	Config     *config_proto.Config
	ACLManager vql_subsystem.ACLManager
	Uploader   api.Uploader
	Logger     *log.Logger
	Env        *ordereddict.Dict
	Repository *Repository
}

func (self ScopeBuilder) Build() *vfilter.Scope {
	return self._build(false)
}

// Only used in tests - this is much more expensive.
func (self ScopeBuilder) BuildFromScratch() *vfilter.Scope {
	return self._build(true)
}

func (self ScopeBuilder) _build(from_scratch bool) *vfilter.Scope {
	env := ordereddict.NewDict()
	if self.Env != nil {
		env.MergeFrom(self.Env)
	}

	if self.Repository == nil {
		self.Repository, _ = GetGlobalRepository(self.Config)
	}

	env.Set(constants.SCOPE_CONFIG, self.Config.Client).
		Set(constants.SCOPE_SERVER_CONFIG, self.Config).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	if self.ACLManager != nil {
		env.Set(vql_subsystem.ACL_MANAGER_VAR, self.ACLManager)
	}

	if self.Uploader != nil {
		env.Set(constants.SCOPE_UPLOADER, self.Uploader)
	}

	scope := MakeScope(self.Repository, from_scratch).AppendVars(env)
	scope.Logger = self.Logger

	return scope
}

func ScopeBuilderFromScope(scope *vfilter.Scope) *ScopeBuilder {
	result := &ScopeBuilder{
		Logger: scope.Logger,
	}
	config_obj, ok := GetServerConfig(scope)
	if ok {
		result.Config = config_obj
	}

	uploader, ok := GetUploader(scope)
	if ok {
		result.Uploader = uploader
	}

	acl_manger, ok := GetACLManager(scope)
	if ok {
		result.ACLManager = acl_manger
	}

	return result
}

// Gets the config from the scope.
func GetConfig(scope *vfilter.Scope) (*config_proto.ClientConfig, bool) {
	scope_config, pres := scope.Resolve(constants.SCOPE_CONFIG)
	if !pres {
		return nil, false
	}

	config, ok := scope_config.(*config_proto.ClientConfig)
	if config == nil {
		return nil, false
	}
	return config, ok
}

func GetServerConfig(scope *vfilter.Scope) (*config_proto.Config, bool) {
	scope_config, pres := scope.Resolve(constants.SCOPE_SERVER_CONFIG)
	if !pres {
		return nil, false
	}

	config, ok := scope_config.(*config_proto.Config)
	return config, ok
}

func GetUploader(scope *vfilter.Scope) (api.Uploader, bool) {
	scope_uploader, pres := scope.Resolve(constants.SCOPE_UPLOADER)
	if !pres {
		return nil, false
	}

	config, ok := scope_uploader.(api.Uploader)
	return config, ok
}

func GetACLManager(scope *vfilter.Scope) (vql_subsystem.ACLManager, bool) {
	scope_manager, pres := scope.Resolve(vql_subsystem.ACL_MANAGER_VAR)
	if !pres {
		return nil, false
	}

	config, ok := scope_manager.(vql_subsystem.ACLManager)
	return config, ok
}
