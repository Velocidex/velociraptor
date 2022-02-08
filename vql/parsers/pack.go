// +build xXXX

package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vtypes"
)

type PackFunctionArg struct {
	Item vfilter.Any `vfilter:"required,field=item,doc=The item to pack."`
	Type string      `vfilter:"required,field=type,doc=The type to pack as (uint8,uint16,uint32,uint64 etc)."`
}
type PackFunction struct{}

func (self PackFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "pack",
		Doc:     "Pack an item into a binary string.",
		ArgType: type_map.AddType(scope, &ParseBinaryFunctionArg{}),
	}
}

func (self ParseBinaryFunction) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &ParseBinaryFunctionArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_binary: %v", err)
		return &vfilter.Null{}
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("parse_binary: %s", err)
		return &vfilter.Null{}
	}

	// Compile the profile and cache it in the scope for next time.
	profile, ok := vql_subsystem.CacheGet(scope, arg.Profile).(*vtypes.Profile)
	if !ok {
		profile = vtypes.NewProfile()
		vtypes.AddModel(profile)

		// Parse the profile.
		err := profile.ParseStructDefinitions(arg.Profile)
		if err != nil {
			scope.Log("parse_binary: %s", err)
			return &vfilter.Null{}
		}
		vql_subsystem.CacheSet(scope, arg.Profile, profile)
	}

	lru_size := vql_subsystem.GetIntFromRow(scope, scope, constants.BINARY_CACHE_SIZE)
	paged_reader, err := readers.NewPagedReader(
		scope, arg.Accessor, arg.Filename, int(lru_size))
	if err != nil {
		scope.Log("parse_binary: %v", err)
		return &vfilter.Null{}
	}

	obj, err := profile.Parse(scope, arg.Struct, paged_reader, arg.Offset)
	if err != nil {
		scope.Log("parse_binary: %v", err)
		return &vfilter.Null{}
	}

	return obj
}

func init() {
	vql_subsystem.RegisterProtocol(&vtypes.StructAssociative{})
	vql_subsystem.RegisterProtocol(&vtypes.ArrayAssociative{})
	vql_subsystem.RegisterProtocol(&vtypes.ArrayIterator{})
	vql_subsystem.RegisterFunction(&ParseBinaryFunction{})
}
