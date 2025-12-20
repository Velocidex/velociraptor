package accessors

import (
	"context"
	"fmt"
	"reflect"

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

	return ParseOSPath(ctx, scope, accessor, value)
}

func ParseOSPath(ctx context.Context,
	scope types.Scope, accessor FileSystemAccessor,
	value interface{}) (*OSPath, error) {

	switch t := value.(type) {
	case types.LazyExpr:
		return ParseOSPath(ctx, scope, accessor, t.ReduceWithScope(ctx, scope))

	case types.Materializer:
		return ParseOSPath(ctx, scope, accessor, t.Materialize(ctx, scope))

	case *OSPath:
		return t, nil

	case *PathSpec:
		root, err := accessor.ParsePath("")
		if err != nil {
			return accessor.ParsePath(t.String())
		}
		return root, root.SetPathSpec(t)

	case PathSpec:

		root, err := accessor.ParsePath("")
		if err != nil {
			return accessor.ParsePath(t.String())
		}
		return root, root.SetPathSpec(&t)

	case api.FSPathSpec:
		// Create an OSPath to represent the abstract filestore path.
		// Restore the file extension from the filestore abstract
		// pathspec.
		components := utils.CopySlice(t.Components())
		if len(components) > 0 {
			last_idx := len(components) - 1
			components[last_idx] += api.GetExtensionForFilestore(t)
		}
		res := MustNewFileStorePath("fs:").Append(components...)

		// Store the FSPathSpec in the data for fast retrieval if we
		// are passed to the fs accessor (this is commonly the case).
		res.Data = t

		return res, nil

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
		result, _ := accessor.ParsePath("")

		// Is it an array? Generic code to handle arrays - just append
		// each element together to form a single path. This allows
		// joining components directly:
		//     ["bin", "ls"] or ["/usr/bin", "ls"]
		a_value := reflect.Indirect(reflect.ValueOf(value))
		if a_value.Type().Kind() == reflect.Slice {
			for idx := 0; idx < a_value.Len(); idx++ {
				slice_item := a_value.Index(int(idx)).Interface()
				item, err := ParseOSPath(ctx, scope, accessor, slice_item)
				if err != nil {
					string_item, ok := slice_item.(string)
					if ok {
						result = result.Append(string_item)
					}
					continue
				}
				result = result.Append(item.Components...)
			}
			return result, nil
		}

		// This is a fatal error on the client.
		return nil, fmt.Errorf("Expecting a path arg type, not %T", t)
	}
}

func parseOSPathArray(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict,
	value interface{}) (interface{}, error) {

	result := []*OSPath{}

	a_value := reflect.Indirect(reflect.ValueOf(value))
	if a_value.Type().Kind() == reflect.Slice {
		for idx := 0; idx < a_value.Len(); idx++ {
			item, err := parseOSPath(ctx, scope, args,
				a_value.Index(int(idx)).Interface())
			if err != nil {
				continue
			}
			item_os_path, ok := item.(*OSPath)
			if ok {
				result = append(result, item_os_path)
			}
		}
		return result, nil
	}

	// If the arg is not a slice then treat it as a single ospath.
	item, err := parseOSPath(ctx, scope, args, value)
	if err != nil {
		return nil, err
	}

	item_os_path, ok := item.(*OSPath)
	if ok {
		result = append(result, item_os_path)
	}
	return result, nil
}

func init() {
	arg_parser.RegisterParser(&OSPath{}, parseOSPath)
	arg_parser.RegisterParser([]*OSPath{}, parseOSPathArray)
}
