package orgs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type CurrentOrgFunction struct{}

func (self *CurrentOrgFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := services.RequireFrontend()
	if err != nil {
		scope.Log("org: %v", err)
		return vfilter.Null{}
	}

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

	org_config_obj, _ := org_manager.GetOrgConfig(config_obj.OrgId)
	client_config := &config_proto.Config{
		Version: org_config_obj.Version,
		Client:  org_config_obj.Client,
	}

	return ordereddict.NewDict().
		Set("name", org_record.Name).
		Set("nonce", org_record.Nonce).
		Set("id", config_obj.OrgId).
		Set("_client_config", client_config)
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
