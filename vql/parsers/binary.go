package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/readers"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vtypes"
)

// VQL bindings to binary parsing.

type ParseBinaryFunctionArg struct {
	Filename string `vfilter:"required,field=filename,doc=Binary file to open."`
	Accessor string `vfilter:"optional,field=accessor,doc=The accessor to use"`
	Profile  string `vfilter:"optional,field=profile,doc=Profile to use (see https://github.com/Velocidex/vtypes)."`
	Struct   string `vfilter:"required,field=struct,doc=Name of the struct in the profile to instantiate."`
	Offset   int64  `vfilter:"optional,field=offset,doc=Start parsing from this offset"`
}
type ParseBinaryFunction struct{}

func (self ParseBinaryFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_binary",
		Doc:     "Parse a binary file into a datastructure using a profile.",
		ArgType: type_map.AddType(scope, &ParseBinaryFunctionArg{}),
	}
}

func (self ParseBinaryFunction) Call(
	ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &ParseBinaryFunctionArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
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

	paged_reader := readers.NewPagedReader(scope, arg.Accessor, arg.Filename)
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
