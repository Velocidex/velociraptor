package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/users"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type UsersPlugin struct{}

func (self UsersPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("users: %s", err)
			return
		}

		any_config_obj, _ := scope.Resolve(constants.SCOPE_SERVER_CONFIG)
		config_obj, ok := any_config_obj.(*config_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		user_list, err := users.ListUsers(config_obj)
		if err != nil {
			scope.Log("users: %v", err)
			return
		}

		for _, user_details := range user_list {
			user_details.PasswordHash = nil
			user_details.PasswordSalt = nil
			policy, err := acls.GetPolicy(
				config_obj, user_details.Name)
			if err == nil {
				user_details.Permissions = policy
			}
			output_chan <- user_details.VelociraptorUser
		}

	}()

	return output_chan
}

func (self UsersPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "gui_users",
		Doc:  "Retrieve the list of users on the server.",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&UsersPlugin{})
}
