package hunts

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type HuntInfoFunctionArg struct {
	HuntId string `vfilter:"optional,field=hunt_id,doc=Hunt Id to look up or a flow id created by that hunt (e.g. F.CRUU3KIE5D73G.H )."`
}

type HuntInfoFunction struct{}

func (self *HuntInfoFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
	if err != nil {
		scope.Log("hunt_info: %s", err)
		return &vfilter.Null{}
	}

	arg := &HuntInfoFunctionArg{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("hunt_info: %v", err)
		return &vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("hunt_info: %v", err)
		return &vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("hunt_info: Command can only run on the server")
		return &vfilter.Null{}
	}

	if strings.HasSuffix(arg.HuntId, ".H") &&
		strings.HasPrefix(arg.HuntId, "F.") {
		arg.HuntId = "H." + strings.TrimSuffix(
			strings.TrimPrefix(arg.HuntId, "F."), ".H")
	}

	hunt_dispatcher_service, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		scope.Log("hunt_info: %v", err)
		return &vfilter.Null{}
	}

	hunt_obj, pres := hunt_dispatcher_service.GetHunt(ctx, arg.HuntId)
	if !pres {
		return &vfilter.Null{}
	}

	return json.ConvertProtoToOrderedDict(hunt_obj)
}

func (self HuntInfoFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "hunt_info",
		Doc:      "Retrieve the hunt information.",
		ArgType:  type_map.AddType(scope, &HuntInfoFunctionArg{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&HuntInfoFunction{})
}
