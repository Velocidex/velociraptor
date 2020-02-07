package server

import (
	"context"
	"path"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/grpc_client"
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
		RowType: type_map.AddType(scope, &flows_proto.ArtifactCollectorContext{}),
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

type CancelFlowFunction struct{}

func (self *CancelFlowFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &FlowsPluginArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("cancel_flow: %s", err.Error())
		return vfilter.Null{}
	}

	any_config_obj, _ := scope.Resolve("server_config")
	config_obj, ok := any_config_obj.(*config_proto.Config)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	res, err := flows.CancelFlow(config_obj, arg.ClientId, arg.FlowId, "VQL query",
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
		arg := &FlowsPluginArgs{}
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("enumerate_flow: %s", err.Error())
			return
		}

		any_config_obj, _ := scope.Resolve("server_config")
		config_obj, ok := any_config_obj.(*config_proto.Config)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		file_store_factory := file_store.GetFileStore(config_obj)

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

		// Delete all the uploads from the flow.
		csv_file_path := artifacts.GetUploadsMetadata(
			arg.ClientId, arg.FlowId)
		defer emit("UploadMetadata", csv_file_path)

		fd, err := file_store_factory.ReadFile(csv_file_path)
		if err == nil {
			defer fd.Close()

			for row := range csv.GetCSVReader(fd) {
				upload, pres := row.Get("vfs_path")
				if pres {
					emit("Upload", upload.(string))
				}
			}
		}

		repository, _ := artifacts.GetGlobalRepository(config_obj)
		for _, full_artifact_name := range collection_context.ArtifactsWithResults {
			artifact_name, source := artifacts.SplitFullSourceName(full_artifact_name)
			artifact, pres := repository.Get(artifact_name)
			if !pres {
				scope.Log("Artifact %s not known", artifact_name)
				continue
			}

			csv_path := artifacts.GetCSVPath(
				arg.ClientId, "*",
				arg.FlowId, artifact_name, source,
				artifacts.ModeNameToMode(artifact.Type))

			globber := make(glob.Globber)
			accessor := file_store.GetFileStoreFileSystemAccessor(config_obj)
			globber.Add(csv_path, accessor.PathSplit)

			// Expanding the glob is not sorted but we really need
			// to go in order of dates.
			for hit := range globber.ExpandWithContext(ctx, "", accessor) {
				full_path := hit.FullPath()
				emit("Result", full_path)
			}

		}

		// The flow's logs
		log_path := path.Join(collection_context.Urn, "logs")
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
