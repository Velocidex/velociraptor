package tools

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/config"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	ClientRestart = make(chan string)
)

type RekeyFunctionArgs struct {
	Wait int64 `vfilter:"optional,field=wait,doc=Wait this long before rekeying the client."`
}

type RekeyFunction struct{}

func (self *RekeyFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &RekeyFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("rekey: %v", err)
		return vfilter.Null{}
	}

	// This is a privileged operation
	err = vql_subsystem.CheckAccess(scope, acls.EXECVE)
	if err != nil {
		scope.Log("rekey: %v", err)
		return vfilter.Null{}
	}

	// Check the config if we are allowed to execve at all.
	config_obj, ok := artifacts.GetConfig(scope)
	if !ok || config_obj == nil {
		scope.Log("rekey: Must be running on a client to rekey %v", config_obj)
		return vfilter.Null{}
	}

	writeback, err := config.GetWriteback(config_obj)
	if err != nil {
		scope.Log("rekey: %v", err)
		return vfilter.Null{}
	}

	pem, err := crypto_utils.GeneratePrivateKey()
	if err != nil {
		scope.Log("rekey: %v", err)
		return vfilter.Null{}
	}

	private_key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr(pem)
	if err != nil {
		scope.Log("rekey: %v", err)
		return vfilter.Null{}
	}

	// Update the write back.
	writeback.PrivateKey = string(pem)
	err = config.UpdateWriteback(config_obj, writeback)
	if err != nil {
		scope.Log("rekey: %v", err)
		return vfilter.Null{}
	}

	new_client_id := crypto_utils.ClientIDFromPublicKey(&private_key.PublicKey)

	// Send the new client id to the main client loop so it can
	// restart, but wait a bit to allow messages to be sent to the
	// server on the old client id.
	go func() {
		time.Sleep(time.Duration(arg.Wait) * time.Second)

		select {
		case ClientRestart <- new_client_id:
		default:
		}
	}()
	return new_client_id
}

func (self RekeyFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "rekey",
		Doc:     "Causes the client to rekey and regenerate a new client ID. DANGEROUS! This will change the client's identity and it will appear as a new client in the GUI.",
		ArgType: type_map.AddType(scope, &RekeyFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&RekeyFunction{})
}
