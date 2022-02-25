package accessors

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

// Parse a value into an OSPath. This is used by VQL functions to
// accept an OSPath object from VQL as an argument. If the argument is
// already a *OSPath then we dont need to do anything and we just
// reuse it saving us the effort of serializing and unserializing the
// same thing. We also accept a string path and automatically convert
// it to an OSPath.
func parseOSPath(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict,
	value interface{}) (interface{}, error) {

	accessor_name := arg_parser.GetStringArg(ctx, scope, args, "accessor")
	accessor, err := GetAccessor(accessor_name, scope)
	if err != nil {
		return nil, err
	}

	switch t := value.(type) {
	case types.LazyExpr:
		return parseOSPath(ctx, scope, args, t.ReduceWithScope(ctx, scope))

	case *OSPath:
		return t, nil

	case api.FSPathSpec:
		// Create an OSPath to represent the abstract filestore path.
		// Restore the file extension from the filestore abstract
		// pathspec.
		components := utils.CopySlice(t.Components())
		if len(components) > 0 {
			last_idx := len(components) - 1
			components[last_idx] += api.GetExtensionForFilestore(t)
		}
		return MustNewFileStorePath("fs:").Append(components...), nil

	case api.DSPathSpec:
		// Create an OSPath to represent the abstract filestore path.
		// Restore the file extension from the filestore abstract
		// pathspec.
		components := utils.CopySlice(t.Components())
		if len(components) > 0 {
			last_idx := len(components) - 1
			components[last_idx] += api.GetExtensionForDatastore(t)
		}
		return MustNewFileStorePath("ds:").Append(components...), nil

	case string:
		return accessor.ParsePath(t)

	case []uint8:
		return accessor.ParsePath(string(t))

	default:
		return nil, fmt.Errorf("Expecting a path arg type, not %T", t)
	}
}

func ParseOSPath(ctx context.Context,
	scope types.Scope, accessor FileSystemAccessor,
	value interface{}) (*OSPath, error) {

	switch t := value.(type) {
	case types.LazyExpr:
		return ParseOSPath(ctx, scope, accessor, t.ReduceWithScope(ctx, scope))

	case *OSPath:
		return t, nil

	case string:
		return accessor.ParsePath(t)

	case []uint8:
		return accessor.ParsePath(string(t))

	default:
		return nil, fmt.Errorf("Expecting a path arg type, not %T", t)
	}
}

func init() {
	arg_parser.RegisterParser(&OSPath{}, parseOSPath)
}
