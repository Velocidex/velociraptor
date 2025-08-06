package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vtypes"
)

// VQL bindings to binary parsing.

type ParseBinaryFunctionArg struct {
	Filename *accessors.OSPath `vfilter:"required,field=filename,doc=Binary file to open."`
	Accessor string            `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Profile  string            `vfilter:"optional,field=profile,doc=Profile to use (see https://github.com/Velocidex/vtypes)."`
	Struct   string            `vfilter:"required,field=struct,doc=Name of the struct in the profile to instantiate."`
	Offset   int64             `vfilter:"optional,field=offset,doc=Start parsing from this offset"`
}
type ParseBinaryFunction struct{}

func (self ParseBinaryFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "parse_binary",
		Doc:      "Parse a binary file into a datastructure using a profile.",
		ArgType:  type_map.AddType(scope, &ParseBinaryFunctionArg{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func (self ParseBinaryFunction) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "parse_binary", args)()

	arg := &ParseBinaryFunctionArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_binary: %v", err)
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
	paged_reader, err := readers.NewAccessorReader(
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
	for _, p := range vtypes.GetProtocols() {
		vql_subsystem.RegisterProtocol(p)
	}
	vql_subsystem.RegisterFunction(&ParseBinaryFunction{})
}
