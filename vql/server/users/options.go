package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type UserOptionsFunctionArgs struct {
	Username        string              `vfilter:"required,field=user,doc=The user to create or update."`
	Theme           string              `vfilter:"optional,field=theme,doc=Set the user's theme."`
	Timezone        string              `vfilter:"optional,field=timezone,doc=Set the user's timezone."`
	Lang            string              `vfilter:"optional,field=lang,doc=Set the user's language."`
	Org             string              `vfilter:"optional,field=org,doc=Set the user's default org id."`
	Links           vfilter.StoredQuery `vfilter:"optional,field=links,doc=Set the user's default links. This should be a list of dicts with columns: type, text, url, icon_url, new_tab, encode, parameter, method, disabled."`
	DefaultPassword string              `vfilter:"optional,field=default_password,doc=Set the user's default password for Zip Exports."`
}

type UserOptionsFunction struct{}

func (self UserOptionsFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	// ACLs are checked by the users module
	arg := &UserOptionsFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_options: %s", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("user_options: %v", err)
		return vfilter.Null{}
	}

	org_config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("user_options: Command can only run on the server")
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	users_manager := services.GetUserManager()

	if arg.DefaultPassword != "" ||
		arg.Theme != "" ||
		arg.Timezone != "" ||
		arg.Lang != "" ||
		arg.Org != "" ||
		!utils.IsNil(arg.Links) {

		request := &api_proto.SetGUIOptionsRequest{
			Theme:           arg.Theme,
			Timezone:        arg.Timezone,
			Lang:            arg.Lang,
			DefaultPassword: arg.DefaultPassword,
			Org:             arg.Org,
		}

		if !utils.IsNil(arg.Links) {
			for _, row := range types.Materialize(ctx, scope, arg.Links) {
				item := &config_proto.GUILink{
					Text:      vql_subsystem.GetStringFromRow(scope, row, "text"),
					Url:       vql_subsystem.GetStringFromRow(scope, row, "url"),
					IconUrl:   vql_subsystem.GetStringFromRow(scope, row, "icon_url"),
					Type:      vql_subsystem.GetStringFromRow(scope, row, "type"),
					NewTab:    vql_subsystem.GetBoolFromRow(scope, row, "new_tab"),
					Encode:    vql_subsystem.GetStringFromRow(scope, row, "encode"),
					Parameter: vql_subsystem.GetStringFromRow(scope, row, "parameter"),
					Method:    vql_subsystem.GetStringFromRow(scope, row, "method"),
					Disabled:  vql_subsystem.GetBoolFromRow(scope, row, "disabled"),
				}
				request.Links = append(request.Links, item)
			}
		}

		err = users_manager.SetUserOptions(ctx, principal, arg.Username, request)
		if err != nil {
			scope.Log("user_options: %v", err)
			return vfilter.Null{}
		}

		err = services.LogAudit(ctx,
			org_config_obj, principal, "user_options",
			ordereddict.NewDict().
				Set("username", arg.Username).
				Set("request", request))
		if err != nil {
			logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
			logger.Error("<red>user_options</> %v %v", principal, arg.Username)
		}
	}

	options, err := users_manager.GetUserOptions(ctx, arg.Username)
	if err != nil {
		scope.Log("user_options: %v", err)
		return vfilter.Null{}
	}

	return options
}

func (self UserOptionsFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "user_options",
		Doc:     "Update and read the user options",
		ArgType: type_map.AddType(scope, &UserOptionsFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UserOptionsFunction{})
}
