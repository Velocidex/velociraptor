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
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type KillClientFunctionArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
}

type KillClientFunction struct{}

func (self *KillClientFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("killkillkill: %s", err)
		return vfilter.Null{}
	}

	arg := &KillClientFunctionArgs{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("killkillkill: %s", err.Error())
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("killkillkill: %s", err.Error())
		return vfilter.Null{}
	}

	// Queue a cancellation message to the client for this flow
	// id.
	err = db.QueueMessageForClient(config_obj, arg.ClientId,
		&crypto_proto.GrrMessage{
			KillKillKill: &crypto_proto.Cancel{},
			SessionId:    constants.MONITORING_WELL_KNOWN_FLOW,
		})
	if err != nil {
		scope.Log("killkillkill: %s", err.Error())
		return vfilter.Null{}
	}

	err = services.GetNotifier().NotifyListener(config_obj, arg.ClientId)
	if err != nil {
		scope.Log("killkillkill: %s", err.Error())
		return vfilter.Null{}
	}

	return arg.ClientId
}

func (self KillClientFunction) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "killkillkill",
		Doc:     "Kills the client and forces a restart - this is very aggresive!",
		ArgType: type_map.AddType(scope, &KillClientFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&KillClientFunction{})
}
