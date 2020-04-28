package server

import (
	"context"
	"path"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type FlowsPluginArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
	FlowId   string `vfilter:"optional,field=flow_id"`
}

type FlowsPlugin struct{}

func (self FlowsPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("flows: %s", err)
			return
		}

		arg := &FlowsPluginArgs{}
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("flows: %v", err)
			return
		}

		config_obj, ok := artifacts.GetServerConfig(scope)
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

		if arg.FlowId != "" {
			sender(arg.FlowId, arg.ClientId)
			vfilter.ChargeOp(scope)
			return
		}

		urn := path.Dir(flows.GetCollectionPath(arg.ClientId, "X"))
		flow_urns, err := db.ListChildren(config_obj, urn, 0, 10000)
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		for _, child_urn := range flow_urns {
			sender(path.Base(child_urn), arg.ClientId)
			vfilter.ChargeOp(scope)
		}
	}()

	return output_chan
}

func (self FlowsPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "flows",
		Doc:     "Retrieve the flows launched on each client.",
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

type CancelFlowFunction struct{}

func (self *CancelFlowFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_CLIENT)
	if err != nil {
		scope.Log("flows: %s", err)
		return vfilter.Null{}
	}

	arg := &FlowsPluginArgs{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("cancel_flow: %s", err.Error())
		return vfilter.Null{}
	}

	config_obj, ok := artifacts.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	res, err := flows.CancelFlow(ctx, config_obj,
		arg.ClientId, arg.FlowId, "VQL query",
		grpc_client.GRPCAPIClient{})
	if err != nil {
		scope.Log("cancel_flow: %s", err.Error())
		return vfilter.Null{}
	}

	return res
}

func (self CancelFlowFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "cancel_flow",
		Doc:     "Cancels the flow.",
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

type EnumerateFlowPlugin struct{}

func (self EnumerateFlowPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("flows: %s", err)
			return
		}

		arg := &FlowsPluginArgs{}
		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("enumerate_flow: %s", err.Error())
			return
		}

		config_obj, ok := artifacts.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		emit := func(item_type, target string) {
			output_chan <- ordereddict.NewDict().
				Set("Type", item_type).
				Set("VFSPath", target)
		}

		collection_context, err := flows.LoadCollectionContext(
			config_obj, arg.ClientId, arg.FlowId)
		if err != nil {
			scope.Log("enumerate_flow: %v", err)
			return
		}

		flow_path_manager := paths.NewFlowPathManager(
			arg.ClientId, arg.FlowId)

		upload_metadata_path, _ := flow_path_manager.UploadMetadata().
			GetPathForWriting()

		defer emit("UploadMetadata", upload_metadata_path)

		row_chan, err := file_store.GetTimeRange(ctx, config_obj,
			flow_path_manager.UploadMetadata(), 0, 0)
		if err != nil {
			scope.Log("enumerate_flow: %v", err)
			return
		}

		for row := range row_chan {
			upload, pres := row.GetString("vfs_path")
			if pres {
				emit("Upload", upload)
			}
		}

		for _, artifact_name := range collection_context.ArtifactsWithResults {
			result_path, err := result_sets.NewArtifactPathManager(
				config_obj, arg.ClientId, arg.FlowId, artifact_name).
				GetPathForWriting()
			if err != nil {
				scope.Log("enumerate_flow: %v", err)
				continue
			}
			emit("Result", result_path)

		}

		// The flow's logs
		log_path, _ := flow_path_manager.Log().GetPathForWriting()
		emit("Log", log_path)

		// The flow's metadata
		emit("CollectionContext", flows.GetCollectionPath(arg.ClientId,
			arg.FlowId)+".db")
	}()

	return output_chan
}

func (self EnumerateFlowPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "enumerate_flow",
		Doc:     "Enumerate all the files that make up a flow.",
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&EnumerateFlowPlugin{})
	vql_subsystem.RegisterFunction(&CancelFlowFunction{})
	vql_subsystem.RegisterPlugin(&FlowsPlugin{})
}
