package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UsersPluginArgs struct {
	AllOrgs bool `vfilter:"optional,field=all_orgs,doc=If set we enumberate permission for all orgs, otherwise just for this org."`
}

type UsersPlugin struct{}

func (self UsersPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		// Access checks are done by the users module.

		arg := &UsersPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("users: %v", err)
			return
		}

		principal := vql_subsystem.GetPrincipal(scope)

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("users: Command can only run on the server")
			return
		}

		orgs := users.LIST_ALL_ORGS
		if !arg.AllOrgs {
			// Only list the current org.
			orgs = []string{config_obj.OrgId}
		}

		user_list, err := users.ListUsers(ctx, principal, orgs)
		if err != nil {
			scope.Log("users: %v", err)
			return
		}

		org_manager, err := services.GetOrgManager()
		if err != nil {
			scope.Log("users: %v", err)
			return
		}

		for _, user_details := range user_list {
			utils.Debug(user_details)
			for _, org_record := range user_details.Orgs {
				org_config_obj, err := org_manager.GetOrgConfig(org_record.Id)
				if err != nil {
					continue
				}

				details := ordereddict.NewDict().
					Set("name", user_details.Name).
					Set("org_id", org_record.Id).
					Set("org_name", org_record.Name).
					Set("picture", user_details.Picture).
					Set("email", user_details.VerifiedEmail)
				policy, err := services.GetPolicy(org_config_obj, user_details.Name)
				if err == nil {
					details.Set("roles", policy.Roles)
				} else {
					details.Set("roles", &vfilter.Null{})
				}

				effective_policy, err := services.GetEffectivePolicy(org_config_obj, user_details.Name)
				if err == nil {
					details.Set("effective_policy", ConvertPolicyToOrderedDict(effective_policy))
				} else {
					details.Set("effective_policy", &vfilter.Null{})
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
	for _, k := range policy_dict.Keys() {
		v, _ := policy_dict.Get(k)

		switch t := v.(type) {
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

		result.Set(k, v)
	}

	return result
}

func containsOrgId(orgs []*api_proto.Org, org_id string) bool {
	for _, o := range orgs {
		if o.Id == org_id {
			return true
		}
	}
	return false
}
