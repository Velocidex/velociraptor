package path_specs

import (
	"slices"
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

// Builds a filestore pathspec from a plain components list.
//
// A PathSpec contains a list of components **and** a type, so just a
// list of components is not sufficient to infer the type. This
// function relied on internal knowledge of the filestore structure to
// infer the correct type from the component list.
func FromGenericComponentList(components []string) api.FSPathSpec {
	components, path_type := getTypeFromComponents(components)
	return NewUnsafeFilestorePath(components...).SetType(path_type)
}

var (
	anyPrefixes = [][]string{
		[]string{"public"},
		[]string{"backups"},
		[]string{"temp"},

		// Uploaded collections from the client.
		[]string{"clients", "", "collections", "", "uploads"},

		// Notebooks: attachments and uploads
		[]string{"notebooks", "", "attach"},
		[]string{"notebooks", "", "", "uploads"},

		// Client notebooks
		[]string{"clients", "", "collections", "", "notebook", "", "attach"},
		[]string{"clients", "", "collections", "", "notebook", "", "", "uploads"},

		// Client monitoring notebooks
		[]string{"clients", "", "monitoring_notebooks", "", "attach"},
		[]string{"clients", "", "monitoring_notebooks", "", "", "uploads"},

		// Hunt notebooks
		[]string{"hunts", "", "notebook", "", "attach"},
		[]string{"hunts", "", "notebook", "", "", "uploads"},
	}
)

// Returns true if the components address a path which is untyped.
func IsComponentUntyped(components []string) bool {
	return MatchComponentPattern(components, anyPrefixes)
}

func MatchComponentPattern(components []string, patterns [][]string) bool {
	// Everything under the public path is untyped.
	for _, prefix := range patterns {
		if matchPrefix(components, prefix) {
			return true
		}
	}
	return false
}

func getTypeFromComponents(components []string) ([]string, api.PathType) {
	if len(components) == 0 || IsComponentUntyped(components) {
		return components, api.PATH_TYPE_FILESTORE_ANY
	}

	// Client uploads are all untyped
	if len(components) > 4 && components[0] == "clients" {
		return components, api.PATH_TYPE_FILESTORE_ANY
	}

	last_component := components[len(components)-1]

	// Fallback, use the extension to deduce the type.
	fs_type, name := api.GetFileStorePathTypeFromExtension(last_component)

	clone := slices.Clone(components[:len(components)-1])
	return append(clone, name), fs_type
}

func matchPrefix(components []string, prefix []string) bool {
	if len(components) < len(prefix) {
		return false
	}

	for idx, m := range prefix {
		if m != "" && components[idx] != m {
			return false
		}
	}

	return true
}

// Converts a typed pathspec to an untyped pathspec. This is required
// when using in a context that will ignore file extensions.
func ToAnyType(in api.FSPathSpec) api.FSPathSpec {
	if in.Type() != api.PATH_TYPE_FILESTORE_ANY {
		return NewUnsafeFilestorePath(AsGenericComponentList(in)...).
			SetType(api.PATH_TYPE_FILESTORE_ANY)
	}
	return in
}
