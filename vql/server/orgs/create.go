package orgs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type OrgCreateFunctionArgs struct {
	OrgName string `vfilter:"required,field=name,doc=The name of the org."`
	OrgId   string `vfilter:"optional,field=org_id,doc=An ID for the new org (if not set use a random ID)."`
}

type OrgCreateFunction struct{}

func (self OrgCreateFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.ORG_ADMIN)
	if err != nil {
		scope.Log("org_create: %s", err)
		return vfilter.Null{}
	}

	arg := &OrgCreateFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("org_create: %s", err)
		return vfilter.Null{}
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		scope.Log("org_create: %s", err)
		return vfilter.Null{}
	}

	org_record, err := org_manager.CreateNewOrg(arg.OrgName, arg.OrgId)
	if err != nil {
		scope.Log("org_create: %s", err)
		return vfilter.Null{}
	}

	return org_record
}

func (self OrgCreateFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "org_create",
		Doc:     "Creates a new organization.",
		ArgType: type_map.AddType(scope, &OrgCreateFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&OrgCreateFunction{})
}
