package api

import (
	"path/filepath"
	"runtime"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

// A safe path into the data store. This path can not consist of any
// user settable attributes and so we never escape it. Typically these
// paths are obtained from path manager objects that orchestrate the
// datastore schema. If the path may consist of user settable
// components, the path manager will return the full components list
// and that API.

// This design strikes a balance between:

// 1. Avoids the need for any other code to worry about filestore
//    encoding since encoding happens only at the file store -
//    everything else just passes the raw component slice around.

// 2. Avoids the need to encode/decode paths that are fixed and safe
//    such as paths created for internal use with already known safe
//    names.

// Ultimately the path managers determine which API is most
// appropriate by either offering a path or a component list.

// An unsafe path comes from user input. We will sanitize components
// in order to come up with the safe path for the filesystem.
type UnsafeDatastorePath struct {
	components []string
	extension  string
}

func (self UnsafeDatastorePath) Base() string {
	return self.components[len(self.components)-1]
}

func (self UnsafeDatastorePath) Components() []string {
	return self.components
}

// Adds an unsafe component to this path
func (self UnsafeDatastorePath) AddChild(child ...string) UnsafeDatastorePath {
	return UnsafeDatastorePath{
		components: append(utils.CopySlice(self.components), child...),
		extension:  self.extension,
	}
}

// Adds the file extension to the last component. Makes a copy of the
// slice.
func (self UnsafeDatastorePath) SetFileExtension(ext string) UnsafeDatastorePath {
	return UnsafeDatastorePath{
		components: self.components,
		extension:  ext,
	}
}

func (self UnsafeDatastorePath) AsDatastoreDirectory(
	config_obj *config_proto.Config) string {

	sep := string(filepath.Separator)
	result := utils.JoinComponents(self.components, sep)

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators. Main's
	// validateServerConfig() ensures this is the case.
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + config_obj.Datastore.Location + sep + result
	}
	return config_obj.Datastore.Location + sep + result
}

func (self UnsafeDatastorePath) AsDatastoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) + self.extension + ".db"
}

type SafeDatastorePath struct {
	components []string
	extension  string
}

func NewSafeDatastorePath(path_components ...string) SafeDatastorePath {
	// For windows OS paths need to have a prefix of \\?\
	// (e.g. \\?\C:\Windows) in order to enable long file names.
	result := SafeDatastorePath{
		components: path_components,
	}

	return result
}

// A SafeDatastorePath can be safely converted to an unsafe one. Since
// the safe path guarantees there is no need to escape any componentsh
// (components do not contain special characters).
func (self SafeDatastorePath) AsUnsafe() UnsafeDatastorePath {
	return UnsafeDatastorePath{
		components: self.components,
		extension:  self.extension,
	}
}

func (self SafeDatastorePath) AddComponent(child ...string) UnsafeDatastorePath {
	return self.AsUnsafe().AddChild(child...)
}

func (self SafeDatastorePath) Components() []string {
	return self.components
}

func (self SafeDatastorePath) Base() string {
	return self.components[len(self.components)-1]
}

// It is a json components if it ends with a json extension.
func (self SafeDatastorePath) IsJSON() bool {
	return self.extension == ".json"
}

func (self SafeDatastorePath) IsEmpty() bool {
	return len(self.components) == 0
}

func (self SafeDatastorePath) SetFileExtension(ext string) SafeDatastorePath {
	return SafeDatastorePath{
		components: self.components,
		extension:  ext,
	}
}

func (self SafeDatastorePath) AddChild(child ...string) SafeDatastorePath {
	new_components := make([]string, 0, len(self.components)+len(child))
	new_components = append(new_components, self.components...)
	new_components = append(new_components, child...)

	return SafeDatastorePath{
		components: new_components,
	}
}

// Filename relative to the root of the datastore
func (self SafeDatastorePath) AsRelativeFilename() string {
	sep := string(filepath.Separator)
	return sep + strings.Join(self.components, sep)
}

func (self SafeDatastorePath) AsDatastoreDirectory(
	config_obj *config_proto.Config) string {

	sep := string(filepath.Separator)
	result := strings.Join(self.components, sep)

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators. Main's
	// validateServerConfig() ensures this is the case.
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + config_obj.Datastore.Location + sep + result
	}
	return config_obj.Datastore.Location + sep + result
}

func (self SafeDatastorePath) AsDatastoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) + self.extension + ".db"
}

func (self SafeDatastorePath) AsFilestoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) + self.extension
}
