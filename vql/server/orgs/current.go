package orgs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type CurrentOrgFunction struct{}

func (self *CurrentOrgFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("org: Command can only run on the server")
		return vfilter.Null{}
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		scope.Log("org: %v", err)
		return vfilter.Null{}
	}

	org_record, err := org_manager.GetOrg(config_obj.OrgId)
	if err != nil {
		scope.Log("org: %v", err)
		return vfilter.Null{}
	}

	return org_record
}

func (self CurrentOrgFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "org",
		Doc:     "Return the details of the current org.",
		ArgType: type_map.AddType(scope, &CurrentOrgFunction{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&CurrentOrgFunction{})
}
