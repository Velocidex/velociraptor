package actions

import (
	"errors"
	"path/filepath"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
)

/* This is a minimalistic implementation of GRR's VFSOpen.

Since we do not intend to use GRR's file operations extensively, we do
not support the full pathspec here. The supported subset include:

- Only support OS paths
- Ignore the PathOptions for case insensitive names - we just use the
  standard OS provided case sensitivity.
*/
func GetPathFromPathSpec(pathspec *actions_proto.PathSpec) (*string, error) {
	// TODO: handle windows pathspecs
	components := []string{"/"}
	i := pathspec
	for {
		if i.Pathtype != actions_proto.PathSpec_OS {
			return nil, errors.New("Only supports OS paths.")
		}

		components = append(components, i.Path)

		if i.NestedPath == nil {
			break
		} else {
			i = i.NestedPath
		}
	}

	full_path := filepath.Join(components...)
	return &full_path, nil
}

func LastPathspec(pathspec *actions_proto.PathSpec) *actions_proto.PathSpec {
	i := pathspec
	for {
		if i.NestedPath == nil {
			return i
		} else {
			i = i.NestedPath
		}
	}
}

func CopyPathspec(pathspec *actions_proto.PathSpec) *actions_proto.PathSpec {
	result := *pathspec
	if pathspec.NestedPath != nil {
		result.NestedPath = CopyPathspec(pathspec.NestedPath)
	}

	return &result
}
