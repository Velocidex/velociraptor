package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UploadsPluginsArgs struct {
	ClientId string `vfilter:"optional,field=client_id,doc=The client id to extract"`
	FlowId   string `vfilter:"optional,field=flow_id,doc=A flow ID (client or server artifacts)"`
	HuntId   string `vfilter:"optional,field=hunt_id,doc=A hunt ID"`
}

type UploadsPlugins struct{}

func (self UploadsPlugins) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("uploads: %s", err)
			return
		}

		arg := &UploadsPluginsArgs{}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		ParseUploadArgsFromScope(arg, scope)

		// Allow the plugin args to override the environment scope.
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("uploads: %v", err)
			return
		}

		if arg.HuntId == "" {
			readFlowUploads(ctx, config_obj, scope, output_chan,
				arg.ClientId, arg.FlowId)
			return
		}

		// Handle hunts
		hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
		if err != nil {
			scope.Log("uploads: %v", err)
			return
		}

		for flow_details := range hunt_dispatcher.GetFlows(
			ctx, config_obj, scope, arg.HuntId, 0) {
			client_id := flow_details.Context.ClientId
			flow_id := flow_details.Context.SessionId

			tmp_chan := make(chan vfilter.Row)

			go func() {
				defer close(tmp_chan)

				readFlowUploads(ctx, config_obj, scope, tmp_chan,
					client_id, flow_id)
			}()

			for row := range tmp_chan {
				row_dict, ok := row.(*ordereddict.Dict)
				if !ok {
					continue
				}

				row_dict.Set("ClientId", client_id).
					Set("FlowId", flow_id)

				select {
				case <-ctx.Done():
					return
				case output_chan <- row_dict:
				}
			}
		}
	}()

	return output_chan
}

func readFlowUploads(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	output_chan chan vfilter.Row,
	client_id, flow_id string) {

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, flow_path_manager.UploadMetadata())
	if err != nil {
		scope.Log("uploads: %v", err)
		return
	}
	defer reader.Close()

	for row := range reader.Rows(ctx) {
		vfs_path, pres := row.GetString("vfs_path")
		if !pres {
			continue
		}

		var components []string
		var pathspec api.FSPathSpec

		// The we have the components we get the file store path
		// from there.
		components_any, ok := row.Get("_Components")
		if ok {
			components = utils.ConvertToStringSlice(components_any)
		}

		if len(components) > 0 {
			pathspec = path_specs.NewUnsafeFilestorePath(components...).
				SetType(api.PATH_TYPE_FILESTORE_ANY)

			row.Set("client_path", vfs_path)
		} else {
			// Each row is the full filestore path of the upload.
			pathspec = path_specs.NewUnsafeFilestorePath(
				utils.SplitComponents(vfs_path)...).
				SetType(api.PATH_TYPE_FILESTORE_ANY)
		}

		row.Update("vfs_path", pathspec)

		select {
		case <-ctx.Done():
			return
		case output_chan <- row:
		}
	}
}

func (self UploadsPlugins) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "uploads",
		Doc:      "Retrieve information about a flow's uploads.",
		ArgType:  type_map.AddType(scope, &UploadsPluginsArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func ParseUploadArgsFromScope(arg *UploadsPluginsArgs, scope vfilter.Scope) {
	client_id, pres := scope.Resolve("ClientId")
	if pres {
		arg.ClientId, _ = client_id.(string)
	}

	flow_id, pres := scope.Resolve("FlowId")
	if pres {
		arg.FlowId, _ = flow_id.(string)
	}

	hunt_id, pres := scope.Resolve("HuntId")
	if pres {
		arg.HuntId, _ = hunt_id.(string)
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&UploadsPlugins{})
}
