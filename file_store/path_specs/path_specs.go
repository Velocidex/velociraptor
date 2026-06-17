package path_specs

import (
	"strconv"
	"sync"

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

	mu                 sync.Mutex
	cached_client_path *string
}

func (self *DSPathSpec) String() string {
	return "ds:" + self.AsClientPath()
}

func (self *DSPathSpec) IsDir() bool {
	return self.is_dir
}

func (self *DSPathSpec) IsSafe() bool {
	return self.is_safe
}

func (self *DSPathSpec) SetDir() api.DSPathSpec {
	self.is_dir = true
	self.path_type = api.PATH_TYPE_DATASTORE_DIRECTORY

	return self
}

func (self *DSPathSpec) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote("ds:" + self.AsClientPath())), nil
}

func (self *DSPathSpec) Base() string {
	if len(self.components) == 0 {
		return ""
	}
	return self.components[len(self.components)-1]
}

func (self *DSPathSpec) Tag() string {
	return self.tag
}

func (self *DSPathSpec) SetTag(tag string) api.DSPathSpec {
	self.tag = tag
	return self
}

func (self *DSPathSpec) Dir() api.DSPathSpec {
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

func (self *DSPathSpec) Components() []string {
	return self.components
}

func (self *DSPathSpec) Type() api.PathType {
	return self.path_type
}

// Adds an unsafe component to this path.
func (self *DSPathSpec) AddChild(child ...string) api.DSPathSpec {
	return &DSPathSpec{
		components: append(utils.CopySlice(self.components), child...),
		path_type:  self.path_type,
		is_safe:    self.is_safe,
	}
}

func (self *DSPathSpec) AddUnsafeChild(child ...string) api.DSPathSpec {
	return &DSPathSpec{
		components: append(utils.CopySlice(self.components), child...),
		path_type:  self.path_type,
		is_safe:    false,
	}
}

func (self *DSPathSpec) SetType(ext api.PathType) api.DSPathSpec {
	return &DSPathSpec{
		components: self.components,
		path_type:  ext,
		is_safe:    self.is_safe,
	}
}

func (self *DSPathSpec) AsClientPath() string {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.cached_client_path != nil {
		return *self.cached_client_path
	}

	res := utils.JoinComponents(self.components, "/") +
		api.GetExtensionForDatastore(self)
	self.cached_client_path = &res
	return res
}

func (self *DSPathSpec) AsFilestorePath() api.FSPathSpec {
	return &FSPathSpec{DSPathSpec: &DSPathSpec{
		components: self.components,
		path_type:  api.PATH_TYPE_FILESTORE_JSON,
		is_safe:    self.is_safe,
	}}
}

func NewUnsafeDatastorePath(path_components ...string) api.DSPathSpec {
	result := &DSPathSpec{
		components: path_components,
		// By default write JSON files.
		path_type: api.PATH_TYPE_DATASTORE_JSON,
		is_safe:   false,
	}

	return result
}

func NewSafeDatastorePath(path_components ...string) api.DSPathSpec {
	result := &DSPathSpec{
		components: path_components,
		path_type:  api.PATH_TYPE_DATASTORE_JSON,
		is_safe:    true,
	}

	return result
}
