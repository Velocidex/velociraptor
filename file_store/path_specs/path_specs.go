package path_specs

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

type DSPathSpec struct {
	components []string
	path_type  api.PathType

	// If this is true we can avoid sanitizing the path on writing
	// to the filestore.
	is_safe bool
}

func (self DSPathSpec) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(self.AsClientPath())), nil
}

func (self DSPathSpec) Base() string {
	return self.components[len(self.components)-1]
}

func (self DSPathSpec) Dir() api.DSPathSpec {
	new_components := utils.CopySlice(self.components)
	if len(new_components) > 0 {
		new_components = new_components[:len(new_components)-1]
	}
	return &DSPathSpec{
		components: new_components,
		path_type:  self.path_type,
	}
}

func (self DSPathSpec) Components() []string {
	return self.components
}

func (self DSPathSpec) Type() api.PathType {
	return self.path_type
}

func (self DSPathSpec) AsSafe() api.DSPathSpec {
	new_components := make([]string, 0, len(self.components))
	for _, i := range self.components {
		new_components = append(new_components, utils.SanitizeString(i))
	}

	return DSPathSpec{
		components: new_components,
		path_type:  self.path_type,
		is_safe:    true,
	}
}

// Adds an unsafe component to this path.
func (self DSPathSpec) AddChild(child ...string) api.DSPathSpec {
	return DSPathSpec{
		components: append(utils.CopySlice(self.components), child...),
		path_type:  self.path_type,
		is_safe:    self.is_safe,
	}
}

func (self DSPathSpec) AddUnsafeChild(child ...string) api.DSPathSpec {
	return DSPathSpec{
		components: append(utils.CopySlice(self.components), child...),
		path_type:  self.path_type,
		is_safe:    false,
	}
}

func (self DSPathSpec) SetType(ext api.PathType) api.DSPathSpec {
	return DSPathSpec{
		components: self.components,
		path_type:  ext,
		is_safe:    self.is_safe,
	}
}

func (self DSPathSpec) AsClientPath() string {
	return utils.JoinComponents(self.components, "/")
}

func (self DSPathSpec) AsDatastoreDirectory(
	config_obj *config_proto.Config) string {
	if self.is_safe {
		return self.asSafeDirWithRoot(config_obj.Datastore.Location)
	}
	return self.asUnsafeDirWithRoot(config_obj.Datastore.Location)
}

// When we are unsafe we need to Sanitize components hitting the
// filesystem.
func (self DSPathSpec) asUnsafeDirWithRoot(root string) string {
	sep := string(filepath.Separator)
	new_components := make([]string, 0, len(self.components))
	for _, i := range self.components {
		new_components = append(new_components, utils.SanitizeString(i))
	}
	result := strings.Join(new_components, sep)

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators. Main's
	// validateServerConfig() ensures this is the case.
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + root + result
	}
	return root + result
}

func (self DSPathSpec) AsDatastoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) +
		api.GetExtensionForDatastore(self, self.path_type)
}

func (self DSPathSpec) AsFilestorePath() api.FSPathSpec {
	return &FSPathSpec{DSPathSpec{
		components: self.components,
		path_type:  api.PATH_TYPE_FILESTORE_JSON,
		is_safe:    self.is_safe,
	}}
}

func NewUnsafeDatastorePath(path_components ...string) DSPathSpec {
	result := DSPathSpec{
		components: path_components,
		// By default write JSON files.
		path_type: api.PATH_TYPE_DATASTORE_JSON,
		is_safe:   false,
	}

	return result
}

func NewSafeDatastorePath(path_components ...string) DSPathSpec {
	result := DSPathSpec{
		components: path_components,
		path_type:  api.PATH_TYPE_DATASTORE_JSON,
		is_safe:    true,
	}

	return result
}

// If the path spec is already safe we can shortcut it and not
// sanitize.
func (self DSPathSpec) asSafeDirWithRoot(root string) string {
	// No need to sanitize here because the DSPathSpec is already
	// safe.
	sep := string(filepath.Separator)
	result := strings.Join(self.components, sep)

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators, and having a trailing
	// \. Main's config.ValidateDatastoreConfig() ensures this is
	// the case.
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + root + result
	}
	return root + result
}
