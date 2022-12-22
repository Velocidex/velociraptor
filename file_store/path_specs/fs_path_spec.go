package path_specs

import (
	"strconv"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

type FSPathSpec struct {
	DSPathSpec

	tag string
}

func (self FSPathSpec) Tag() string {
	return self.tag
}

func (self FSPathSpec) SetTag(tag string) api.FSPathSpec {
	self.tag = tag
	return self
}

func (self FSPathSpec) String() string {
	return "fs:" + self.AsClientPath()
}

func (self FSPathSpec) Dir() api.FSPathSpec {
	new_components := utils.CopySlice(self.components)
	if len(new_components) > 0 {
		new_components = new_components[:len(new_components)-1]
	}
	return FSPathSpec{DSPathSpec: DSPathSpec{
		components: new_components,
		path_type:  self.path_type,
	}}
}

func (self FSPathSpec) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote("fs:" + self.AsClientPath())), nil
}

func (self FSPathSpec) AsDatastorePath() api.DSPathSpec {
	return self.DSPathSpec.
		SetType(api.PATH_TYPE_DATASTORE_JSON)
}

// Adds an unsafe component to this path.
func (self FSPathSpec) AddChild(child ...string) api.FSPathSpec {
	return FSPathSpec{DSPathSpec: DSPathSpec{
		components: append(utils.CopySlice(self.components), child...),
		path_type:  self.path_type,
		is_safe:    self.is_safe,
	}}
}

func (self FSPathSpec) AddUnsafeChild(child ...string) api.FSPathSpec {
	return FSPathSpec{DSPathSpec: DSPathSpec{
		components: append(utils.CopySlice(self.components), child...),
		path_type:  self.path_type,
		is_safe:    false,
	}}
}

func (self FSPathSpec) SetType(ext api.PathType) api.FSPathSpec {
	return FSPathSpec{DSPathSpec: DSPathSpec{
		components: self.components,
		path_type:  ext,
		is_safe:    self.is_safe,
	}}
}

func (self FSPathSpec) AsFilestoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsFilestoreDirectory(config_obj) +
		api.GetExtensionForFilestore(self)
}

func (self FSPathSpec) AsFilestoreDirectory(
	config_obj *config_proto.Config) string {
	data_store_root := ""
	if config_obj != nil && config_obj.Datastore != nil {
		data_store_root = config_obj.Datastore.FilestoreDirectory
	}

	if self.is_safe {
		return self.asSafeDirWithRoot(data_store_root)
	}
	return self.asUnsafeDirWithRoot(data_store_root)
}

func (self FSPathSpec) AsClientPath() string {
	return utils.JoinComponents(self.components, "/") +
		api.GetExtensionForFilestore(self)
}

func NewUnsafeFilestorePath(path_components ...string) api.FSPathSpec {
	result := FSPathSpec{DSPathSpec: DSPathSpec{
		components: path_components,
		// By default write JSON files.
		path_type: api.PATH_TYPE_FILESTORE_JSON,
		is_safe:   false,
	}}

	return result
}

func NewSafeFilestorePath(path_components ...string) api.FSPathSpec {
	result := FSPathSpec{DSPathSpec: DSPathSpec{
		components: path_components,
		path_type:  api.PATH_TYPE_FILESTORE_JSON,
		is_safe:    true,
	}}

	return result
}
