package frontend

import (
	"context"
	"fmt"
	"io"

	"github.com/Velocidex/ordereddict"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
	"www.velocidex.com/golang/vfilter/utils/dict"
)

type RelayedFunction struct {
	Delegate types.FunctionInterface

	config_obj *config_proto.Config
}

func (self RelayedFunction) Copy() types.FunctionInterface {
	return RelayedFunction{
		Delegate:   self.Delegate,
		config_obj: self.config_obj,
	}
}

func (self RelayedFunction) name(scope types.Scope) string {
	type_map := types.NewTypeMap()
	info := self.Delegate.Info(scope, type_map)
	return info.Name
}

func (self RelayedFunction) Call(
	ctx context.Context,
	scope types.Scope,
	args *ordereddict.Dict) types.Any {

	name := self.name(scope)
	principal := vql_subsystem.GetPrincipal(scope)

	// Materializes all the args
	args = dict.RowToDict(ctx, scope, args)

	serialized, err := args.MarshalJSON()
	if err != nil {
		scope.Log("%v: %v", name, err)
		return vfilter.Null{}
	}

	// Relay the call to the API server. We make the call as the
	// superuser because we do not have credentials for the user. The
	// server will switch user context on our behalf later.
	client, closer, err := grpc_client.Factory.GetAPIClient(
		ctx, grpc_client.SuperUser, self.config_obj)
	if err != nil {
		scope.Log("%v: %v", name, err)
		return vfilter.Null{}
	}
	defer func() { _ = closer() }()

	org_id := utils.GetOrgId(self.config_obj)
	query_config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if ok {
		org_id = utils.GetOrgId(query_config_obj)
	}

	request := &actions_proto.VQLCollectorArgs{
		OrgId:   org_id,
		MaxRow:  1000,
		MaxWait: 1,
		Env: []*actions_proto.VQLEnv{{
			Key:   "FuncArgs",
			Value: string(serialized),
		}, {
			Key:   "Principal",
			Value: principal,
		}},

		// Use the query plugin to switch user contexts
		Query: []*actions_proto.VQLRequest{{
			VQL: fmt.Sprintf(`
SELECT * FROM query(runas=Principal, query={
 SELECT %v(%v=parse_json(data=FuncArgs)) AS Result
 FROM scope()
}, env=dict(FuncArgs=FuncArgs))
`, name, "`**`"),
		}},
	}

	stream, err := client.Query(ctx, request)
	if err != nil {
		scope.Log("%v: %v", name, err)
		return vfilter.Null{}
	}

	for {
		response, err := stream.Recv()
		if err != nil && err != io.EOF {
			return err
		}

		if response == nil {
			break
		}

		// Relay logs to our own query
		if response.Log != "" {
			scope.Debug("Relayed %s: %s", name, response.Log)
		}

		json_response := response.Response
		if json_response == "" {
			json_response = response.JSONLResponse
		}

		if json_response == "" {
			continue
		}

		rows, err := utils.ParseJsonToDicts([]byte(json_response))
		if err != nil {
			scope.Log("%v: %v", name, err)
			return vfilter.Null{}
		}

		for _, row := range rows {
			result, pres := row.Get("Result")
			if pres {
				// Return the first result
				return result
			}
		}
	}

	return vfilter.Null{}
}

func (self RelayedFunction) Info(
	scope types.Scope, type_map *types.TypeMap) *types.FunctionInfo {
	return self.Delegate.Info(scope, type_map)
}

type RelayedPlugin struct {
	Delegate types.PluginGeneratorInterface

	config_obj *config_proto.Config
}

func (self RelayedPlugin) Copy() types.PluginGeneratorInterface {
	return RelayedPlugin{
		Delegate:   self.Delegate,
		config_obj: self.config_obj,
	}
}

func (self RelayedPlugin) name(scope types.Scope) string {
	type_map := types.NewTypeMap()
	info := self.Delegate.Info(scope, type_map)
	return info.Name
}

func (self RelayedPlugin) Call(
	ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) <-chan types.Row {

	output_chan := make(chan types.Row)

	go func() {
		defer close(output_chan)

		name := self.name(scope)
		principal := vql_subsystem.GetPrincipal(scope)

		// Materializes all the args
		args = dict.RowToDict(ctx, scope, args)

		serialized, err := args.MarshalJSON()
		if err != nil {
			scope.Log("%v: %v", name, err)
			return
		}

		// Relay the call to the API server. We make the call as the
		// superuser because we do not have credentials for the user. The
		// server will switch user context on our behalf later.
		client, closer, err := grpc_client.Factory.GetAPIClient(
			ctx, grpc_client.SuperUser, self.config_obj)
		if err != nil {
			scope.Log("%v: %v", name, err)
			return
		}

		defer func() { _ = closer() }()

		org_id := utils.GetOrgId(self.config_obj)
		query_config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if ok {
			org_id = utils.GetOrgId(query_config_obj)
		}

		request := &actions_proto.VQLCollectorArgs{
			OrgId:   org_id,
			MaxRow:  1000,
			MaxWait: 1,
			Env: []*actions_proto.VQLEnv{{
				Key:   "PluginArgs",
				Value: string(serialized),
			}, {
				Key:   "Principal",
				Value: principal,
			}},

			// Use the query plugin to switch user contexts
			Query: []*actions_proto.VQLRequest{{
				VQL: fmt.Sprintf(`
SELECT * FROM query(runas=Principal, query={
 SELECT * FROM %v(%v=parse_json(data=PluginArgs))
}, env=dict(PluginArgs=PluginArgs))
`, name, "`**`"),
			}},
		}

		stream, err := client.Query(ctx, request)
		if err != nil {
			scope.Log("%v: %v", name, err)
			return
		}

		for {
			response, err := stream.Recv()
			if err != nil && err != io.EOF {
				scope.Log("%v: %v", name, err)
				return
			}

			if response == nil {
				break
			}

			// Relay logs to our own query
			if response.Log != "" {
				scope.Debug("Relayed %s: %s", name, response.Log)
			}

			json_response := response.Response
			if json_response == "" {
				json_response = response.JSONLResponse
			}

			if json_response == "" {
				continue
			}

			rows, err := utils.ParseJsonToDicts([]byte(json_response))
			if err != nil {
				scope.Log("%v: %v", name, err)
				return
			}

			for _, row := range rows {
				select {
				case <-ctx.Done():
					return

				case output_chan <- row:
				}
			}
		}
	}()

	return output_chan
}

func (self RelayedPlugin) Info(
	scope types.Scope, type_map *types.TypeMap) *types.PluginInfo {
	return self.Delegate.Info(scope, type_map)
}
