package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type InventoryAddFunctionArgs struct {
	Tool         string `vfilter:"required,field=tool"`
	ServeLocally bool   `vfilter:"optional,field=serve_locally"`
	URL          string `vfilter:"optional,field=url"`
	Hash         string `vfilter:"optional,field=hash"`
	Filename     string `vfilter:"optional,field=filename"`
}

type InventoryAddFunction struct{}

func (self *InventoryAddFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &InventoryAddFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("inventory_add: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("inventory_add: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := artifacts.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	request := &artifacts_proto.Tool{
		Name:         arg.Tool,
		ServeLocally: arg.ServeLocally,
		Url:          arg.URL,
		Filename:     arg.Filename,
		Hash:         arg.Hash,
	}

	err = services.GetInventory().AddTool(ctx, config_obj, request)
	if err != nil {
		scope.Log("inventory_add: %s", err.Error())
		return vfilter.Null{}
	}

	tool, err := services.GetInventory().GetToolInfo(ctx, config_obj, arg.Tool)
	if err != nil {
		scope.Log("inventory_add: %s", err.Error())
		return vfilter.Null{}
	}

	return tool
}

func (self *InventoryAddFunction) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
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
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &InventoryGetFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("inventory_get: %s", err.Error())
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("inventory_get: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := artifacts.GetServerConfig(scope)
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
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
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
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		for _, item := range services.GetInventory().Get().Tools {
			output_chan <- item
		}

	}()
	return output_chan
}

func (self InventoryPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
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
