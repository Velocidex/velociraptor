package path_specs

import (
	"strings"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

// If child_components are a subpath of parent_components (i.e. are
// parent_components is an exact prefix of child_components)
func IsSubPath(parent api.FSPathSpec, child api.FSPathSpec) bool {
	parent_components := parent.Components()
	child_components := child.Components()

	// Parent path can not be longer than child
	if len(parent_components) > len(child_components) {
		return false
	}

	for i := 0; i < len(parent_components); i++ {
		if parent_components[i] != child_components[i] {
			return false
		}
	}
	return true
}

func DebugPathSpecList(list []api.DSPathSpec) string {
	result := []string{}
	for _, i := range list {
		result = append(result, i.AsClientPath())
	}
	return strings.Join(result, ", ")
}

// Returns a file store path spec as a generic list of components,
// adjusting the filename path extension as necessary.
func AsGenericComponentList(path api.FSPathSpec) []string {
	components := utils.CopySlice(path.Components())
	if len(components) > 0 {
		components[len(components)-1] += api.GetExtensionForFilestore(path)
	}
	return components
}

// Builds a filestore pathspec from a plain components list. Uses the
// extension of the filename component to determine the path type.
func FromGenericComponentList(components []string) api.FSPathSpec {
	pathspec := NewUnsafeFilestorePath(components...)
	if len(components) > 0 {
		last_idx := len(components) - 1
		fs_type, name := api.GetFileStorePathTypeFromExtension(
			components[last_idx])
		return pathspec.Dir().AddChild(name).SetType(fs_type)
	}
	return pathspec
}
