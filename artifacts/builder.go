package artifacts

import (
	"log"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Make it easier to build a query scope using the aritfact
// repository.
type ScopeBuilder struct {
	Config     *config_proto.Config
	ACLManager vql_subsystem.ACLManager
	Uploader   uploads.Uploader
	Logger     *log.Logger
	Env        *ordereddict.Dict
}

func (self ScopeBuilder) Build() *vfilter.Scope {
	if self.Env == nil {
		self.Env = ordereddict.NewDict()
	}

	self.Env.Set(constants.SCOPE_CONFIG, self.Config.Client).
		Set(constants.SCOPE_SERVER_CONFIG, self.Config).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	if self.ACLManager != nil {
		self.Env.Set(vql_subsystem.ACL_MANAGER_VAR, self.ACLManager)
	}

	if self.Uploader != nil {
		self.Env.Set(constants.SCOPE_UPLOADER, self.Uploader)
	}

	repository, err := GetGlobalRepository(self.Config)
	if err != nil {
		panic(err)
	}
	scope := MakeScope(repository).AppendVars(self.Env)
	scope.Logger = self.Logger

	return scope
}
