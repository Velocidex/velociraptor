// +build server_vql

package flows

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type FlowsPluginArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
	FlowId   string `vfilter:"optional,field=flow_id"`
}

type FlowsPlugin struct{}

func (self FlowsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
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
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("flows: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
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

			select {
			case <-ctx.Done():
				return
			case output_chan <- json.ConvertProtoToOrderedDict(collection_context):
			}

		}

		if arg.FlowId != "" {
			sender(arg.FlowId, arg.ClientId)
			vfilter.ChargeOp(scope)
			return
		}

		flow_path_manager := paths.NewFlowPathManager(arg.ClientId, arg.FlowId)
		flow_urns, err := db.ListChildren(
			config_obj, flow_path_manager.ContainerPath())
		if err != nil {
			scope.Log("Error: %v", err)
			return
		}

		for _, child_urn := range flow_urns {
			if !child_urn.IsDir() {
				sender(child_urn.Base(), arg.ClientId)
				vfilter.ChargeOp(scope)
			}
		}
	}()

	return output_chan
}

func (self FlowsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "flows",
		Doc:     "Retrieve the flows launched on each client.",
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

type CancelFlowFunction struct{}

func (self *CancelFlowFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &FlowsPluginArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("cancel_flow: %s", err.Error())
		return vfilter.Null{}
	}

	permissions := acls.COLLECT_CLIENT
	if arg.ClientId == "server" {
		permissions = acls.COLLECT_SERVER
	}

	err = vql_subsystem.CheckAccess(scope, permissions)
	if err != nil {
		scope.Log("cancel_flow: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	res, err := flows.CancelFlow(ctx, config_obj,
		arg.ClientId, arg.FlowId, "VQL query")
	if err != nil {
		scope.Log("cancel_flow: %v", err.Error())
		return vfilter.Null{}
	}

	return json.ConvertProtoToOrderedDict(res)
}

func (self CancelFlowFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "cancel_flow",
		Doc:     "Cancels the flow.",
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

type EnumerateFlowPlugin struct{}

func (self EnumerateFlowPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
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
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("enumerate_flow: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		collection_context, err := flows.LoadCollectionContext(
			config_obj, arg.ClientId, arg.FlowId)
		if err != nil {
			scope.Log("enumerate_flow: %v", err)
			return
		}

		flow_path_manager := paths.NewFlowPathManager(
			arg.ClientId, arg.FlowId)

		upload_metadata_path := flow_path_manager.UploadMetadata()
		r := &reporter{
			ctx: ctx, output_chan: output_chan,
			seen: make(map[string]bool),
		}
		file_store_factory := file_store.GetFileStore(config_obj)
		reader, err := result_sets.NewResultSetReader(
			file_store_factory, flow_path_manager.UploadMetadata())
		if err == nil {
			for row := range reader.Rows(ctx) {
				upload, pres := row.GetString("vfs_path")
				if pres {
					// Each row is the full filestore path of the upload.
					pathspec := path_specs.NewUnsafeFilestorePath(
						utils.SplitComponents(upload)...).
						SetType(api.PATH_TYPE_FILESTORE_ANY)

					r.emit_fs("Upload", pathspec)
				}
			}
		}

		// Order results to facilitate deletion - container deletion
		// happens after we read its contents.
		r.emit_fs("UploadMetadata", upload_metadata_path)
		r.emit_fs("UploadMetadataIndex", upload_metadata_path.
			SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))

		// Remove all result sets from artifacts.
		for _, artifact_name := range collection_context.ArtifactsWithResults {
			path_manager, err := artifact_paths.NewArtifactPathManager(
				config_obj, arg.ClientId, arg.FlowId, artifact_name)
			if err != nil {
				scope.Log("enumerate_flow: %v", err)
				continue
			}

			result_path, err := path_manager.GetPathForWriting()
			if err != nil {
				scope.Log("enumerate_flow: %v", err)
				continue
			}
			r.emit_fs("Result", result_path)
			r.emit_fs("ResultIndex",
				result_path.SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))

		}

		r.emit_fs("Log", flow_path_manager.Log())
		r.emit_fs("LogIndex", flow_path_manager.Log().
			SetType(api.PATH_TYPE_FILESTORE_JSON_INDEX))
		r.emit_ds("CollectionContext", flow_path_manager.Path())
		r.emit_ds("Task", flow_path_manager.Task())

		// Walk the flow's datastore and filestore
		db, err := datastore.GetDB(config_obj)
		if err != nil {
			return
		}

		r.emit_ds("Notebook", flow_path_manager.Notebook().Path())
		db.Walk(config_obj, flow_path_manager.Notebook().DSDirectory(),
			func(path api.DSPathSpec) error {
				r.emit_ds("NotebookData", path)
				return nil
			})

		api.Walk(file_store_factory,
			flow_path_manager.Notebook().Directory(),
			func(path api.FSPathSpec, info os.FileInfo) error {
				r.emit_fs("NotebookItem", path)
				return nil
			})

	}()

	return output_chan
}

type reporter struct {
	ctx         context.Context
	output_chan chan<- vfilter.Row
	seen        map[string]bool
}

func (self *reporter) emit_ds(
	item_type string, target api.DSPathSpec) {
	client_path := target.AsClientPath()

	if self.seen[client_path] {
		return
	}
	self.seen[client_path] = true

	select {
	case <-self.ctx.Done():
		return
	case self.output_chan <- ordereddict.NewDict().
		Set("Type", item_type).
		Set("VFSPath", target):
	}
}

func (self *reporter) emit_fs(
	item_type string, target api.FSPathSpec) {
	client_path := target.AsClientPath()

	if self.seen[client_path] {
		return
	}
	self.seen[client_path] = true

	select {
	case <-self.ctx.Done():
		return
	case self.output_chan <- ordereddict.NewDict().
		Set("Type", item_type).
		Set("VFSPath", target):
	}
}

func (self EnumerateFlowPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
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
