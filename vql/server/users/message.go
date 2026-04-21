package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserMessageFunctionArgs struct {
	Username string            `vfilter:"required,field=user,doc=The user to create or update."`
	Message  *ordereddict.Dict `vfilter:"optional,field=message,doc=The message to deliver to the user."`
}

type UserMessageFunction struct{}

func (self UserMessageFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	// ACLs are checked by the users module
	arg := &UserMessageFunctionArgs{}
	kw, err := arg_parser.ExtractKWArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_message: %s", err)
		return vfilter.Null{}
	}

	if arg.Message == nil {
		arg.Message = kw
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("user_message: %v", err)
		return vfilter.Null{}
	}

	_, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("user_message: Command can only run on the server")
		return vfilter.Null{}
	}

	users_manager := services.GetUserManager()
	principal := vql_subsystem.GetPrincipal(scope)
	err = users_manager.MessageUser(ctx, arg.Username, principal, arg.Message)
	if err != nil {
		scope.Log("user_message: %s", err)
		return vfilter.Null{}
	}

	return arg.Username
}

func (self UserMessageFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:         "user_message",
		Doc:          "Send the user a message which will appear in the user notification view.",
		ArgType:      type_map.AddType(scope, &UserMessageFunctionArgs{}),
		Version:      1,
		FreeFormArgs: true,
	}
}

type UserMessagesArgs struct {
	Clear    bool   `vfilter:"optional,field=clear,doc=If set also clear messages."`
	Username string `vfilter:"optional,field=user,doc=If set we read another user's messages (must be admin)."`
}

type UserMessages struct{}

func (self UserMessages) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "user_messages", args)()

		// Access checks are done by the users module.

		arg := &UserMessagesArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("user_messages: %v", err)
			return
		}

		principal := vql_subsystem.GetPrincipal(scope)
		target_user := principal

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("user_messages: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("user_messages: Command can only run on the server")
			return
		}

		// Only admin can read another user's messages
		if arg.Username != "" && principal != arg.Username {
			err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
			if err != nil {
				scope.Log("user_messages: %v", err)
				return
			}

			target_user = arg.Username
		}

		if arg.Clear {
			defer func() {
				user_manager := services.GetUserManager()
				err = user_manager.SetUserOptions(
					ctx, principal, target_user,
					&api_proto.SetGUIOptionsRequest{
						Messages: -1,
					})
				if err != nil {
					scope.Log("user_messages: %v", err)
				}
			}()
		}

		file_store_factory := file_store.GetFileStore(config_obj)
		path_manager := paths.NewUserPathManager(target_user)
		reader, err := result_sets.NewResultSetReader(file_store_factory,
			path_manager.Notifications())
		if err != nil {
			return
		}

		for row := range reader.Rows(ctx) {
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self UserMessages) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "user_messages",
		Doc:     "Emit the user's console messages.",
		ArgType: type_map.AddType(scope, &UserMessagesArgs{}),
		Version: 1,
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&UserMessages{})
	vql_subsystem.RegisterFunction(&UserMessageFunction{})
}
