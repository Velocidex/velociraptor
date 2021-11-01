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

const (
	WINDOWS_LFN_PREFIX = "\\\\?\\"
)

type DSPathSpec struct {
	components []string
	path_type  api.PathType

	// If this is true we can avoid sanitizing the path on writing
	// to the filestore.
	is_safe bool
	is_dir  bool

	tag string
}

func (self DSPathSpec) String() string {
	return "ds:" + self.AsClientPath()
}

func (self DSPathSpec) IsDir() bool {
	return self.is_dir
}

func (self DSPathSpec) SetDir() api.DSPathSpec {
	self.is_dir = true
	self.path_type = api.PATH_TYPE_DATASTORE_DIRECTORY

	return self
}

func (self DSPathSpec) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote("ds:" + self.AsClientPath())), nil
}

func (self DSPathSpec) Base() string {
	if len(self.components) == 0 {
		return ""
	}
	return self.components[len(self.components)-1]
}

func (self DSPathSpec) Tag() string {
	return self.tag
}

func (self DSPathSpec) SetTag(tag string) api.DSPathSpec {
	self.tag = tag
	return self
}

func (self DSPathSpec) Dir() api.DSPathSpec {
	new_components := utils.CopySlice(self.components)
	if len(new_components) > 0 {
		new_components = new_components[:len(new_components)-1]
	}
	return &DSPathSpec{
		components: new_components,
		is_dir:     true,
		path_type:  api.PATH_TYPE_DATASTORE_DIRECTORY,
	}
}

func (self DSPathSpec) Components() []string {
	return self.components
}

func (self DSPathSpec) Type() api.PathType {
	return self.path_type
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
	return utils.JoinComponents(self.components, "/") +
		api.GetExtensionForDatastore(self)
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
		if i != "" {
			new_components = append(new_components, utils.SanitizeString(i))
		}
	}
	result := sep + strings.Join(new_components, sep)

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
		api.GetExtensionForDatastore(self)
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

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators, and having a trailing
	// \. Main's config.ValidateDatastoreConfig() ensures this is
	// the case.
	if runtime.GOOS == "windows" {
		// Remove empty components which are broken on windows due to
		// the long filename hack.
		components := make([]string, 0, len(self.components))
		for _, c := range self.components {
			if c != "" {
				components = append(components, c)
			}
		}
		return WINDOWS_LFN_PREFIX + root + sep + strings.Join(components, sep)
	}

	return root + sep + strings.Join(self.components, sep)
}
