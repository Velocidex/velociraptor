//go:build windows && amd64 && cgo
// +build windows,amd64,cgo

package process

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DescribeAddressArgs struct {
	RVA        int64  `vfilter:"required,field=rva,doc=The Relative Virtual Address to describe."`
	ModulePath string `vfilter:"required,field=module,doc=The path of the PE file to inspect."`
}

type DescribeAddress struct{}

func (self DescribeAddress) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "describe_address", args)()

	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_READ)
	if err != nil {
		scope.Log("describe_address: %s", err)
		return vfilter.Null{}
	}

	arg := &DescribeAddressArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("describe_address: %s", err.Error())
		return vfilter.Null{}
	}

	kim := GetKernelInfoManager(scope)
	module_path := kim.NormalizeFilename(arg.ModulePath)

	module_file_name := filepath.Base(module_path)

	func_name := kim.GuessFunctionName(module_path, arg.RVA)
	if func_name != "" {
		func_name = fmt.Sprintf("%v!%v", module_file_name, func_name)
	} else {
		func_name = fmt.Sprintf("%v!%#x", module_file_name, arg.RVA)
	}

	return ordereddict.NewDict().
		Set("func", func_name).
		Set("module", module_path)
}

func (self DescribeAddress) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "describe_address",
		Doc:      "Describe an address in the PE text section.",
		ArgType:  type_map.AddType(scope, &DescribeAddressArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&DescribeAddress{})
}
