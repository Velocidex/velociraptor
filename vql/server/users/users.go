package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type UsersPluginArgs struct {
	AllOrgs bool `vfilter:"optional,field=all_orgs,doc=If set we enumerate permission for all orgs, otherwise just for this org."`
}

type UsersPlugin struct{}

func (self UsersPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "users", args)()

		// Access checks are done by the users module.

		arg := &UsersPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("users: %v", err)
			return
		}

		principal := vql_subsystem.GetPrincipal(scope)

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("users: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("users: Command can only run on the server")
			return
		}

		orgs := services.LIST_ALL_ORGS
		if !arg.AllOrgs {
			// Only list the current org.
			orgs = []string{config_obj.OrgId}
		}

		users_manager := services.GetUserManager()
		user_list, err := users_manager.ListUsers(ctx, principal, orgs)
		if err != nil {
			scope.Log("users: %v", err)
			return
		}

		for _, user_details := range user_list {
			for _, org_record := range user_details.Orgs {
				details, err := getUserRecord(ctx, scope,
					org_record.Id, org_record.Name, user_details)
				if err != nil {
					scope.Log("users: %v", err)
					continue
				}
				select {
				case <-ctx.Done():
					return
				case output_chan <- details:
				}
			}
		}

	}()

	return output_chan
}

func (self UsersPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "gui_users",
		Doc:     "Retrieve the list of users on the server.",
		ArgType: type_map.AddType(scope, &UsersPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&UsersPlugin{})
}

func ConvertPolicyToOrderedDict(
	policy *acl_proto.ApiClientACL) *ordereddict.Dict {
	policy_dict := json.ConvertProtoToOrderedDict(policy)
	result := ordereddict.NewDict()
	for _, i := range policy_dict.Items() {
		switch t := i.Value.(type) {
		case bool:
			if !t {
				continue
			}

		case string:
			if t == "" {
				continue
			}

		case []string:
			if len(t) == 0 {
				continue
			}

		case []interface{}:
			if len(t) == 0 {
				continue
			}
		}

		result.Set(i.Key, i.Value)
	}

	return result
}

func getUserRecord(
	ctx context.Context,
	scope vfilter.Scope,
	org_id, org_name string,
	user_details *api_proto.VelociraptorUser) (*ordereddict.Dict, error) {
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(org_id)
	if err != nil {
		return nil, err
	}

	details := ordereddict.NewDict().
		Set("name", user_details.Name).
		Set("org_id", org_id).
		Set("org_name", org_name).
		Set("picture", user_details.Picture).
		Set("email", user_details.VerifiedEmail)
	policy, err := services.GetPolicy(org_config_obj, user_details.Name)
	if err == nil {
		details.Set("roles", policy.Roles)
		details.Set("_policy",
			cleanupDict(scope, vfilter.RowToDict(ctx, scope, policy)))

	} else {
		details.Set("roles", &vfilter.Null{})
		details.Set("_policy", ordereddict.NewDict())
	}

	effective_policy, err := services.GetEffectivePolicy(org_config_obj, user_details.Name)
	if err == nil {
		details.Set("effective_policy", ConvertPolicyToOrderedDict(effective_policy))
	} else {
		details.Set("effective_policy", &vfilter.Null{})
	}
	return details, nil
}

func cleanupDict(scope types.Scope, in *ordereddict.Dict) *ordereddict.Dict {
	result := ordereddict.NewDict()
	for _, i := range in.Items() {
		if scope.Bool(i.Value) {
			result.Set(i.Key, i.Value)
		}
	}
	return result
}
