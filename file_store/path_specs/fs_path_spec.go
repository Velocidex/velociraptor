package path_specs

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FSPathSpec struct {
	DSPathSpec
}

func (self FSPathSpec) Dir() api.FSPathSpec {
	new_components := utils.CopySlice(self.components)
	if len(new_components) > 0 {
		new_components = new_components[:len(new_components)-1]
	}
	return FSPathSpec{DSPathSpec{
		components: new_components,
		path_type:  self.path_type,
	}}
}

func (self FSPathSpec) AsSafe() api.FSPathSpec {
	new_components := make([]string, 0, len(self.components))
	for _, i := range self.components {
		new_components = append(new_components, utils.SanitizeString(i))
	}

	return FSPathSpec{DSPathSpec{
		components: new_components,
		path_type:  self.path_type,
		is_safe:    true,
	}}
}

// Adds an unsafe component to this path.
func (self FSPathSpec) AddChild(child ...string) api.FSPathSpec {
	return FSPathSpec{DSPathSpec{
		components: append(utils.CopySlice(self.components), child...),
		path_type:  self.path_type,
		is_safe:    self.is_safe,
	}}
}

func (self FSPathSpec) AddUnsafeChild(child ...string) api.FSPathSpec {
	return FSPathSpec{DSPathSpec{
		components: append(utils.CopySlice(self.components), child...),
		path_type:  self.path_type,
		is_safe:    false,
	}}
}

func (self FSPathSpec) SetType(ext api.PathType) api.FSPathSpec {
	return FSPathSpec{DSPathSpec{
		components: self.components,
		path_type:  ext,
		is_safe:    self.is_safe,
	}}
}

func (self FSPathSpec) AsFilestoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsFilestoreDirectory(config_obj) +
		api.GetExtensionForFilestore(self, self.path_type)
}

func (self FSPathSpec) AsFilestoreDirectory(
	config_obj *config_proto.Config) string {
	if self.is_safe {
		return self.asSafeDirWithRoot(
			config_obj.Datastore.FilestoreDirectory)
	}
	return self.asUnsafeDirWithRoot(
		config_obj.Datastore.FilestoreDirectory)
}

func NewUnsafeFilestorePath(path_components ...string) FSPathSpec {
	result := FSPathSpec{DSPathSpec{
		components: path_components,
		// By default write JSON files.
		path_type: api.PATH_TYPE_FILESTORE_JSON,
		is_safe:   false,
	}}

	return result
}

func NewSafeFilestorePath(path_components ...string) FSPathSpec {
	result := FSPathSpec{DSPathSpec{
		components: path_components,
		path_type:  api.PATH_TYPE_FILESTORE_JSON,
		is_safe:    true,
	}}

	return result
}
