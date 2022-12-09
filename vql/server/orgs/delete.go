package orgs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
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

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
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

	principal := vql_subsystem.GetPrincipal(scope)
	logging.LogAudit(config_obj, principal, "org_delete",
		logrus.Fields{
			"org_id": arg.OrgId,
		})

	err = org_manager.DeleteOrg(ctx, arg.OrgId)
	if err != nil {
		scope.Log("org_delete: %s", err)
		return vfilter.Null{}
	}

	return arg.OrgId
}

func (self OrgDeleteFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "org_delete",
		Doc:     "Deletes an Org from the server.",
		ArgType: type_map.AddType(scope, &OrgDeleteFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&OrgDeleteFunction{})
}
