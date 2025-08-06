package functions

import (
	"bufio"
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/glaslos/tlsh"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type TLSHashFunctionArgs struct {
	Path     *accessors.OSPath `vfilter:"required,field=path,doc=Path to open and hash."`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
}

type TLSHashFunction struct{}

func (self *TLSHashFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "tlsh_hash", args)()

	arg := &HashFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("tlsh_hash: %v", err)
		return vfilter.Null{}
	}

	cached_buffer := pool.Get().(*[]byte)
	defer pool.Put(cached_buffer)

	fs, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("tlsh_hash: %v", err)
		return vfilter.Null{}
	}

	file, err := fs.Open(arg.Path.String())
	if err != nil {
		return vfilter.Null{}
	}
	defer file.Close()

	tlsh_hash, err := tlsh.HashReader(bufio.NewReader(file))
	if err != nil {
		return vfilter.Null{}
	}

	return tlsh_hash.String()
}

func (self TLSHashFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "tlsh_hash",
		Doc:      "Calculate the tlsh hash of a file.",
		ArgType:  type_map.AddType(scope, &TLSHashFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&TLSHashFunction{})
}
