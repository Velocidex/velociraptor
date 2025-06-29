package server

// Killing the client will wipe the client's ring buffer. We hope the
// client will be restarted by the service manager (this depends on
// the Wix settings: in docs/wix/ we set restart to 30 seconds) so we
// do not lose it.

// Note that generally killing the client is an aggressive action and
// will emit an event log to that effect since it is a hard exit - use
// sparingly.
import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type KillClientFunctionArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
}

type KillClientFunction struct{}

func (self *KillClientFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("killkillkill: %s", err)
		return vfilter.Null{}
	}

	arg := &KillClientFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("killkillkill: %s", err.Error())
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("killkillkill: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("killkillkill: Command can only run on the server")
		return vfilter.Null{}
	}

	// Queue a cancellation message to the client for this flow
	// id.
	client_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		scope.Log("killkillkill: %s", err.Error())
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	err = services.LogAudit(ctx,
		config_obj, principal, "killkillkill",
		ordereddict.NewDict().Set("client_id", arg.ClientId))
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("<red>killkillkill</> %v %v", principal, arg.ClientId)
	}

	err = client_manager.QueueMessageForClient(ctx, arg.ClientId,
		&crypto_proto.VeloMessage{
			KillKillKill: &crypto_proto.Cancel{},
			SessionId:    constants.MONITORING_WELL_KNOWN_FLOW,
		},
		services.NOTIFY_CLIENT, utils.BackgroundWriter)
	if err != nil {
		scope.Log("killkillkill: %s", err.Error())
		return vfilter.Null{}
	}

	return arg.ClientId
}

func (self KillClientFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "killkillkill",
		Doc:      "Kills the client and forces a restart - this is very aggresive!",
		ArgType:  type_map.AddType(scope, &KillClientFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&KillClientFunction{})
}
