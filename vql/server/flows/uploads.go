package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UploadsPluginsArgs struct {
	ClientId string `vfilter:"optional,field=client_id,doc=The client id to extract"`
	FlowId   string `vfilter:"optional,field=flow_id,doc=A flow ID (client or server artifacts)"`
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

		flow_path_manager := paths.NewFlowPathManager(arg.ClientId, arg.FlowId)
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

			// Each row is the full filestore path of the upload.
			pathspec := path_specs.NewUnsafeFilestorePath(
				utils.SplitComponents(vfs_path)...).
				SetType(api.PATH_TYPE_FILESTORE_ANY)

			row.Update("vfs_path", pathspec)

			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self UploadsPlugins) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "uploads",
		Doc:     "Retrieve information about a flow's uploads.",
		ArgType: type_map.AddType(scope, &UploadsPluginsArgs{}),
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
}

func init() {
	vql_subsystem.RegisterPlugin(&UploadsPlugins{})
}
