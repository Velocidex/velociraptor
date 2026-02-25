package filesystem

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type WriteFunctionArgs struct {
	Data        string `vfilter:"required,field=data,doc=The data to write"`
	Destination string `vfilter:"required,field=dest,doc=The destination file to write."`
	Permissions string `vfilter:"optional,field=permissions,doc=Required permissions (e.g. 'x')."`
	Append      bool   `vfilter:"optional,field=append,doc=If true we append to the target file otherwise truncate it"`
	Directories bool   `vfilter:"optional,field=create_directories,doc=If true we ensure the destination directories exist"`
}

type WriteFunction struct{}

// This is basically an alias to the copy() VQL function so we just delegate to that
func (self WriteFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "write_file", args)()

	arg := &WriteFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("write_file: %v", err)
		return vfilter.Null{}
	}

	return CopyFunction{}.Call(ctx, scope, ordereddict.NewDict().
		Set("accessor", "data").
		Set("filename", arg.Data).
		Set("dest", arg.Destination).
		Set("permissions", arg.Permissions).
		Set("append", arg.Append).
		Set("create_directories", arg.Directories))
}

func (self WriteFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "write_file",
		Doc:     "Writes a string onto a file.",
		ArgType: type_map.AddType(scope, &WriteFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(
			acls.FILESYSTEM_WRITE, acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(WriteFunction{})
}
