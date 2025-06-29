package orgs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
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

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("org_create: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("org_create: Command can only run on the server")
		return vfilter.Null{}
	}

	arg := &OrgCreateFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("org_create: %s", err)
		return vfilter.Null{}
	}

	if arg.OrgName == "" {
		scope.Log("ERROR:org_create: An Org name must be specified")
		return vfilter.Null{}
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		scope.Log("org_create: %s", err)
		return vfilter.Null{}
	}

	org_record, err := org_manager.CreateNewOrg(
		arg.OrgName, arg.OrgId, services.RandomNonce)
	if err != nil {
		scope.Log("org_create: %s", err)
		return vfilter.Null{}
	} else if org_record != nil {
		principal := vql_subsystem.GetPrincipal(scope)
		err := services.LogAudit(ctx,
			config_obj, principal, "org_create",
			ordereddict.NewDict().
				Set("name", org_record.Name).
				Set("org_id", org_record.Id).
				Set("nonce", org_record.Nonce))

		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("<red>org_create</> %v %v", principal, org_record.Id)
		}
	}

	return org_record
}

func (self OrgCreateFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "org_create",
		Doc:      "Creates a new organization.",
		ArgType:  type_map.AddType(scope, &OrgCreateFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.ORG_ADMIN).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&OrgCreateFunction{})
}
