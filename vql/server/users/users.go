package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type UsersPlugin struct{}

func (self UsersPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("users: %s", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		users := services.GetUserManager()
		user_list, err := users.ListUsers()
		if err != nil {
			scope.Log("users: %v", err)
			return
		}

		for _, user_details := range user_list {
			policy, err := acls.GetPolicy(
				config_obj, user_details.Name)
			if err == nil {
				user_details.Permissions = policy
			}
			select {
			case <-ctx.Done():
				return
			case output_chan <- json.ConvertProtoToOrderedDict(user_details):
			}
		}

	}()

	return output_chan
}

func (self UsersPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name: "gui_users",
		Doc:  "Retrieve the list of users on the server.",
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&UsersPlugin{})
}
