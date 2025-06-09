//go:build windows
// +build windows

package registry

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/sys/windows/registry"
	registry_accessor "www.velocidex.com/golang/velociraptor/accessors/registry"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type RegSetValueFunctionArgs struct {
	Path   string         `vfilter:"required,field=path,doc=Registry value path."`
	Value  types.LazyExpr `vfilter:"required,field=value,doc=Value to set"`
	Type   string         `vfilter:"required,field=type,doc=Type to set (SZ, DWORD, QWORD)"`
	Create bool           `vfilter:"optional,field=create,doc=Set to create missing intermediate keys"`
}

type RegSetValueFunction struct{}

func (self *RegSetValueFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "reg_set_value", args)()

	arg := &RegSetValueFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("reg_set_value: %s", err.Error())
		return vfilter.Null{}
	}

	// We are about to write on the filesystem - make sure the user
	// has write access.
	err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("reg_set_value: %s", err.Error())
		return vfilter.Null{}
	}

	value := arg.Value.Reduce(ctx)
	components := utils.SplitComponents(arg.Path)
	if len(components) < 2 {
		scope.Log("reg_set_value: Path must be provided: %s ", arg.Path)
		return vfilter.Null{}
	}

	last_idx := len(components) - 1
	value_name := components[last_idx]
	subkey_path := strings.Join(components[1:last_idx], "\\")

	root_hive, ok := registry_accessor.GetHiveFromName(components[0])
	if !ok {
		scope.Log("reg_set_value: Unknown root hive name %s", components[0])
		return vfilter.Null{}
	}

	scope.Log("Setting value %v in key %v in root %v", value_name,
		subkey_path, components[0])

	var key registry.Key

	if arg.Create {
		key, _, err = registry.CreateKey(root_hive, subkey_path,
			registry.QUERY_VALUE|registry.SET_VALUE)

	} else {
		key, err = registry.OpenKey(root_hive, subkey_path,
			registry.QUERY_VALUE|registry.SET_VALUE)
	}
	if err != nil {
		scope.Log("reg_set_value: %s", err.Error())
		return vfilter.Null{}
	}
	defer key.Close()

	if value_name == "@" {
		value_name = ""
	}

	switch arg.Type {
	case "SZ":
		err = key.SetStringValue(value_name, utils.ToString(value))

	case "EXPAND_SZ":
		err = key.SetExpandStringValue(value_name, utils.ToString(value))

	case "BINARY":
		err = key.SetBinaryValue(value_name, []byte(utils.ToString(value)))

	case "DWORD":
		value_int, ok := utils.ToInt64(value)
		if !ok {
			return vfilter.Null{}
		}
		err = key.SetDWordValue(value_name, uint32(value_int))

	case "QWORD":
		value_int, ok := utils.ToInt64(value)
		if !ok {
			return vfilter.Null{}
		}
		err = key.SetQWordValue(value_name, uint64(value_int))

	default:
		scope.Log("reg_set_value: Invalid registry value type %s", arg.Type)
		return vfilter.Null{}
	}

	if err != nil {
		scope.Log("reg_set_value:  %v", err)
		return vfilter.Null{}
	}

	return true
}

func (self RegSetValueFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "reg_set_value",
		Doc:      "Set a value in the registry.",
		ArgType:  type_map.AddType(scope, &RegSetValueFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

type RegDeleteValueFunctionArgs struct {
	Path string `vfilter:"required,field=path,doc=Registry value path."`
}

type RegDeleteValueFunction struct{}

func (self *RegDeleteValueFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "reg_rm_value", args)()

	arg := &RegDeleteValueFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("reg_rm_value: %s", err.Error())
		return vfilter.Null{}
	}

	// We are about to write on the filesystem - make sure the user
	// has write access.
	err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("reg_rm_value: %s", err.Error())
		return vfilter.Null{}
	}

	components := utils.SplitComponents(arg.Path)
	if len(components) < 2 {
		scope.Log("reg_rm_value: Path must be provided: %s ", arg.Path)
		return vfilter.Null{}
	}

	last_idx := len(components) - 1
	value_name := components[last_idx]
	subkey_path := strings.Join(components[1:last_idx], "\\")

	root_hive, ok := registry_accessor.GetHiveFromName(components[0])
	if !ok {
		scope.Log("reg_rm_value: Unknown root hive name %s", components[0])
		return vfilter.Null{}
	}

	scope.Log("Deleting value %v in key %v in root %v", value_name,
		subkey_path, components[0])

	key, err := registry.OpenKey(root_hive, subkey_path,
		registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		scope.Log("reg_rm_value: %s", err.Error())
		return vfilter.Null{}
	}
	defer key.Close()

	err = key.DeleteValue(value_name)
	if err != nil {
		scope.Log("reg_rm_value:  %v", err)
		return vfilter.Null{}
	}

	return true
}

func (self RegDeleteValueFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "reg_rm_value",
		Doc:      "Removes a value in the registry.",
		ArgType:  type_map.AddType(scope, &RegDeleteValueFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

type RegDeleteKeyFunctionArgs struct {
	Path string `vfilter:"required,field=path,doc=Registry key path."`
}

type RegDeleteKeyFunction struct{}

func (self *RegDeleteKeyFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "reg_rm_key", args)()

	arg := &RegDeleteKeyFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("reg_rm_key: %s", err.Error())
		return vfilter.Null{}
	}

	// We are about to write on the filesystem - make sure the user
	// has write access.
	err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("reg_rm_key: %s", err.Error())
		return vfilter.Null{}
	}

	components := utils.SplitComponents(arg.Path)
	if len(components) < 2 {
		scope.Log("reg_rm_key: Path must be provided: %s ", arg.Path)
		return vfilter.Null{}
	}

	subkey_path := strings.Join(components[1:], "\\")

	root_hive, ok := registry_accessor.GetHiveFromName(components[0])
	if !ok {
		scope.Log("reg_rm_key: Unknown root hive name %s", components[0])
		return vfilter.Null{}
	}

	scope.Log("Deleting key %v in root %v", subkey_path, components[0])

	// Open the relevant hive.
	key, err := registry.OpenKey(root_hive, "",
		registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		scope.Log("reg_rm_key: %s", err.Error())
		return vfilter.Null{}
	}
	defer key.Close()

	err = registry.DeleteKey(key, subkey_path)
	if err != nil {
		scope.Log("reg_rm_key:  %v", err)
		return vfilter.Null{}
	}

	return true
}

func (self RegDeleteKeyFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "reg_rm_key",
		Doc:      "Removes a key and all its values from the registry.",
		ArgType:  type_map.AddType(scope, &RegDeleteKeyFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&RegSetValueFunction{})
	vql_subsystem.RegisterFunction(&RegDeleteValueFunction{})
	vql_subsystem.RegisterFunction(&RegDeleteKeyFunction{})
}
