package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type InventoryAddFunctionArgs struct {
	Tool         string `vfilter:"required,field=tool"`
	ServeLocally bool   `vfilter:"optional,field=serve_locally"`
	URL          string `vfilter:"optional,field=url"`
	Hash         string `vfilter:"optional,field=hash"`
	Filename     string `vfilter:"optional,field=filename,doc=The name of the file on the endpoint"`

	File     string `vfilter:"optional,field=file,doc=An optional file to upload"`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use to read the file."`
}

type InventoryAddFunction struct{}

func (self *InventoryAddFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &InventoryAddFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("inventory_add: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("inventory_add: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	tool := &artifacts_proto.Tool{
		Name:         arg.Tool,
		ServeLocally: arg.ServeLocally,
		Url:          arg.URL,
		Filename:     arg.Filename,
		Hash:         arg.Hash,
	}

	if arg.File != "" {
		accessor, err := glob.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("inventory_add: %s", err)
			return vfilter.Null{}
		}

		reader, err := accessor.Open(arg.File)
		if err != nil {
			scope.Log("inventory_add: %s", err)
			return vfilter.Null{}
		}

		path_manager := paths.NewInventoryPathManager(config_obj, tool)
		file_store_factory := file_store.GetFileStore(config_obj)
		writer, err := file_store_factory.WriteFile(path_manager.Path())
		if err != nil {
			scope.Log("inventory_add: %s", err)
			return vfilter.Null{}
		}
		defer writer.Close()

		_ = writer.Truncate()

		sha_sum := sha256.New()

		_, err = utils.Copy(ctx, writer, io.TeeReader(reader, sha_sum))
		if err != nil {
			scope.Log("inventory_add: %s", err)
			return vfilter.Null{}
		}

		tool.Hash = hex.EncodeToString(sha_sum.Sum(nil))
		tool.ServeLocally = true

		if tool.Filename == "" {
			tool.Filename = path.Base(arg.File)
		}
	}

	err = services.GetInventory().AddTool(
		config_obj, tool, services.ToolOptions{
			AdminOverride: true,
		})
	if err != nil {
		scope.Log("inventory_add: %s", err.Error())
		return vfilter.Null{}
	}

	// Do not read the tool back - reading the tool back will
	// force it to be materialized (downloaded). It should be
	// possible to add tools without having this immediately
	// downloaded.
	return json.ConvertProtoToOrderedDict(tool)
}

func (self *InventoryAddFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "inventory_add",
		Doc:     "Add tool to ThirdParty inventory.",
		ArgType: type_map.AddType(scope, &InventoryAddFunctionArgs{}),
	}
}

type InventoryGetFunctionArgs struct {
	Tool string `vfilter:"required,field=tool"`
}

type InventoryGetFunction struct{}

func (self *InventoryGetFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &InventoryGetFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("inventory_get: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("inventory_get: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	tool, err := services.GetInventory().GetToolInfo(ctx, config_obj, arg.Tool)
	if err != nil {
		scope.Log("inventory_get: %s", err.Error())
		return vfilter.Null{}
	}

	url := tool.ServeUrl
	if url == "" {
		url = tool.Url
	}

	result := ordereddict.NewDict().
		Set("Tool_"+arg.Tool+"_HASH", tool.Hash).
		Set("Tool_"+arg.Tool+"_FILENAME", tool.Filename).
		Set("Tool_"+arg.Tool+"_URL", url)
	return result
}

func (self *InventoryGetFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "inventory_get",
		Doc:     "Get tool info from inventory service.",
		ArgType: type_map.AddType(scope, &InventoryGetFunctionArgs{}),
	}
}

type InventoryPluginArgs struct{}

type InventoryPlugin struct{}

func (self InventoryPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		for _, item := range services.GetInventory().Get().Tools {
			select {
			case <-ctx.Done():
				return

			case output_chan <- json.ConvertProtoToOrderedDict(item):
			}
		}

	}()
	return output_chan
}

func (self InventoryPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "inventory",
		Doc:     "Retrieve the tools inventory.",
		ArgType: type_map.AddType(scope, &InventoryPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&InventoryAddFunction{})
	vql_subsystem.RegisterFunction(&InventoryGetFunction{})
	vql_subsystem.RegisterPlugin(&InventoryPlugin{})
}
