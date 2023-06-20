package orgs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type OrgDeleteFunctionArgs struct {
	OrgId string `vfilter:"required,field=org,doc=The org ID to delete."`
}

type OrgDeleteFunction struct{}

func (self OrgDeleteFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.ORG_ADMIN)
	if err != nil {
		scope.Log("org_delete: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("org_delete: Command can only run on the server")
		return vfilter.Null{}
	}

	arg := &OrgDeleteFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("org_delete: %s", err)
		return vfilter.Null{}
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		scope.Log("org_delete: %s", err)
		return vfilter.Null{}
	}

	err = org_manager.DeleteOrg(ctx, arg.OrgId)
	if err != nil {
		scope.Log("org_delete: %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	services.LogAudit(ctx,
		config_obj, principal, "org_delete",
		ordereddict.NewDict().
			Set("org_id", arg.OrgId))

	return arg.OrgId
}

func (self OrgDeleteFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "org_delete",
		Doc:      "Deletes an Org from the server.",
		ArgType:  type_map.AddType(scope, &OrgDeleteFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.ORG_ADMIN).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&OrgDeleteFunction{})
}
