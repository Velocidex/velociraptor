package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	prefetch "www.velocidex.com/golang/go-prefetch"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type LZXpressFunctionArgs struct {
	Data string `vfilter:"required,field=data,doc=The lzxpress stream (bytes)"`
}

// The hash fuction calculates a hash of a file. It may be expensive
// so we make it cancelllable.
type LZXpressFunction struct{}

func (self *LZXpressFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.CheckForPanic(scope, "lzxpress_decompress")

	arg := &LZXpressFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("lzxpress_decompress: %v", err)
		return vfilter.Null{}
	}

	decompressed, err := prefetch.LZXpressHuffmanDecompressWithFallback(
		[]byte(arg.Data), len(arg.Data))
	if err != nil {
		scope.Log("lzxpress_decompress: %v", err)
		return vfilter.Null{}
	}

	return decompressed
}

func (self LZXpressFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "lzxpress_decompress",
		Doc:     "Decompress an lzxpress blob.",
		ArgType: type_map.AddType(scope, &LZXpressFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&LZXpressFunction{})
}
