package server

import (
	"context"
	"path"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type FlowsPluginArgs struct {
	ClientId []string `vfilter:"required,field=client_id"`
	FlowId   string   `vfilter:"optional,field=flow_id"`
}

type FlowsPlugin struct{}

func (self FlowsPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &FlowsPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("flows: %v", err)
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*config_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		db, err := datastore.GetDB(config_obj)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		sender := func(flow_id string, client_id string) {
			collection_context, err := flows.LoadCollectionContext(
				config_obj, client_id, flow_id)
			if err != nil {
				scope.Log("Error: %v", err)
				return
			}

			output_chan <- collection_context
		}

		for _, client_id := range arg.ClientId {
			if arg.FlowId != "" {
				sender(arg.FlowId, client_id)
				continue
			}

			urn := path.Dir(flows.GetCollectionPath(client_id, "X"))
			flow_urns, err := db.ListChildren(config_obj, urn, 0, 10000)
			if err != nil {
				scope.Log("Error: %v", err)
				return
			}
			for _, child_urn := range flow_urns {
				sender(path.Base(child_urn), client_id)
				vfilter.ChargeOp(scope)
			}
		}
	}()

	return output_chan
}

func (self FlowsPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "flows",
		Doc:     "Retrieve the flows launched on each client.",
		RowType: type_map.AddType(scope, &flows_proto.ArtifactCollectorContext{}),
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&FlowsPlugin{})
}
