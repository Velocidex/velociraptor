package api

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

/*

# How paths are handled in Velociraptor.

Velociraptor treats paths as a list of components. Each component is a
string which may contain arbitrary bytes - including path separators.

Ultimately these paths need to be encoded into a filesystem backend
which has many rules about the types of characters allowed on them. We
therefore need to apply conversions to make the general path fit in
the fileystem backend restrictions - this process is called
Sanitization.

For example consider a path like: ["a", "b/c"]

The path consists of two components, one of which happens to have a
path separator. Therefore in order to store it on the filesystem we
must sanitize the components and come up with a filesystem path like
this: "/a/b%2Ec/"

Velociraptor paths can come from many different sources but all
internal representations consist of a list of components. For example:

Client path -> paths.UnsafeDatastorePathFromClientPath() -> UnsafeDatastorePath()
path managers -> SafeDatastorePath()

## SafeDatastorePath vs. UnsafeDatastorePath

When encoding to the filesystem, a Velociraptor path needs to be
sanitized. There are some cases where we know this sanitization is not
needed. Namely when the path is created by a path manager and does not
contain user input. In this case we can take a shortcut - we do not
need to sanitize the data when we write the path in the backing
filesystem (because we have full control over the path components).

For example, consider the path to a client's datastore record:
["clients", client_id]

There is no need to escape anything especially here, since we know all
the components to be safe.

Consider this path from the client:
["clients", client_id, "vfs", "ntfs", "\\\\.\\c:\\", "Windows"]

This path might need to be escaped since it comes from the client
(specifically the device name). Therefore we construct an
UnsafeDatastorePath.

We can convert between an UnsafeDatastorePath to a SafeDatastorePath
by simply sanitizing all components.

This design strikes a balance between:

1. Avoids the need for any other code to worry about filestore
   encoding since encoding happens only at the file store -
   everything else just passes the raw component slice around.

2. Avoids the need to encode/decode paths that are fixed and safe
   such as paths created for internal use with already known safe
   names.

Ultimately the path managers determine which API is most appropriate
by either offering a safe or unsafe path spec
*/

type PathType int

const (
	// By default prefer to store datastore items as JSON but some
	// items may still be protobuf when performance is needed.
	PATH_TYPE_DATASTORE_JSON PathType = iota
	PATH_TYPE_DATASTORE_PROTO
	PATH_TYPE_DATASTORE_YAML // Used for artifacts

	PATH_TYPE_FILESTORE_JSON
	PATH_TYPE_FILESTORE_JSON_INDEX
	PATH_TYPE_FILESTORE_JSON_TIME_INDEX

	// Used to write sparse indexes
	PATH_TYPE_FILESTORE_SPARSE_IDX

	// Used to write zip files in the download folder.
	PATH_TYPE_FILESTORE_DOWNLOAD_ZIP
	PATH_TYPE_FILESTORE_DOWNLOAD_REPORT

	// TMP files
	PATH_TYPE_FILESTORE_TMP
	PATH_TYPE_FILESTORE_LOCK
	PATH_TYPE_FILESTORE_CSV

	// Arbitrary extensions.
	PATH_TYPE_FILESTORE_ANY
)

func GetExtensionForDatastore(path_spec PathSpec, t PathType) string {
	switch t {
	case PATH_TYPE_DATASTORE_PROTO:
		return ".db"

	case PATH_TYPE_DATASTORE_JSON:
		return ".json.db"

	case PATH_TYPE_DATASTORE_YAML:
		return ".yaml"
	}
	panic("filestore path used for datastore for " +
		path_spec.AsClientPath())
}

func GetExtensionForFilestore(path_spec PathSpec, t PathType) string {
	switch t {
	case PATH_TYPE_DATASTORE_PROTO, PATH_TYPE_DATASTORE_JSON:
		panic("datastore path used for filestore for " +
			path_spec.AsClientPath())

	case PATH_TYPE_FILESTORE_JSON:
		return ".json"

	case PATH_TYPE_FILESTORE_JSON_INDEX:
		return ".json.index"

	case PATH_TYPE_FILESTORE_JSON_TIME_INDEX:
		return ".json.tidx"

	case PATH_TYPE_FILESTORE_SPARSE_IDX:
		return ".idx"

	case PATH_TYPE_FILESTORE_DOWNLOAD_ZIP:
		return ".zip"

	case PATH_TYPE_FILESTORE_DOWNLOAD_REPORT:
		return ".html"

	case PATH_TYPE_FILESTORE_TMP:
		return ".tmp"

	case PATH_TYPE_FILESTORE_LOCK:
		return ".lock"

	case PATH_TYPE_FILESTORE_CSV:
		return ".csv"

	case PATH_TYPE_FILESTORE_ANY:
		return ""
	}

	panic("Unsupported path type for " + path_spec.AsClientPath())
}

func GetFileStorePathTypeFromExtension(name string) PathType {
	if strings.HasSuffix(name, ".json") {
		return PATH_TYPE_FILESTORE_JSON
	}

	if strings.HasSuffix(name, ".json.index") {
		return PATH_TYPE_FILESTORE_JSON_INDEX
	}

	if strings.HasSuffix(name, ".json.tidx") {
		return PATH_TYPE_FILESTORE_JSON_TIME_INDEX
	}

	if strings.HasSuffix(name, ".idx") {
		return PATH_TYPE_FILESTORE_SPARSE_IDX
	}

	if strings.HasSuffix(name, ".zip") {
		return PATH_TYPE_FILESTORE_DOWNLOAD_ZIP
	}

	return PATH_TYPE_FILESTORE_ANY
}

type PathSpec interface {
	AsDatastoreDirectory(config_obj *config_proto.Config) string
	AsDatastoreFilename(config_obj *config_proto.Config) string
	AsFilestoreFilename(config_obj *config_proto.Config) string
	AsFilestoreDirectory(config_obj *config_proto.Config) string

	// A path suitable to be exchanged with the client.
	AsClientPath() string

	Components() []string
	Base() string
	Dir() PathSpec
	Type() PathType

	SetType(t PathType) PathSpec

	// Adds a child maintaining safety (i.e. for a safe path keeps
	// the path safe, and for unsafe paths keep them as unsafe).
	AddChild(child ...string) PathSpec

	// Adds children but makes sure that the result is unsafe.
	AddUnsafeChild(child ...string) PathSpec
}

type UnsafeDatastorePath struct {
	components []string
	path_type  PathType
}

func (self UnsafeDatastorePath) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(self.AsClientPath())), nil
}

func (self UnsafeDatastorePath) Base() string {
	return self.components[len(self.components)-1]
}

func (self UnsafeDatastorePath) Dir() PathSpec {
	new_components := utils.CopySlice(self.components)
	if len(new_components) > 0 {
		new_components = new_components[:len(new_components)-1]
	}
	return &UnsafeDatastorePath{
		components: new_components,
		path_type:  self.path_type,
	}
}

func (self UnsafeDatastorePath) Components() []string {
	return self.components
}

func (self UnsafeDatastorePath) Type() PathType {
	return self.path_type
}

func (self UnsafeDatastorePath) AsSafe() SafeDatastorePath {
	new_components := make([]string, 0, len(self.components))
	for _, i := range self.components {
		new_components = append(new_components, utils.SanitizeString(i))
	}

	return SafeDatastorePath{
		components: new_components,
		path_type:  self.path_type,
	}
}

// Adds an unsafe component to this path.
func (self UnsafeDatastorePath) AddChild(child ...string) PathSpec {
	return UnsafeDatastorePath{
		components: append(utils.CopySlice(self.components), child...),
		path_type:  self.path_type,
	}
}

func (self UnsafeDatastorePath) AddUnsafeChild(child ...string) PathSpec {
	return self.AddChild(child...)
}

func (self UnsafeDatastorePath) SetType(ext PathType) PathSpec {
	return UnsafeDatastorePath{
		components: self.components,
		path_type:  ext,
	}
}

func (self UnsafeDatastorePath) AsClientPath() string {
	return utils.JoinComponents(self.components, "/")
}

func (self UnsafeDatastorePath) AsDatastoreDirectory(
	config_obj *config_proto.Config) string {
	return self.asDirWithRoot(config_obj.Datastore.Location)
}

func (self UnsafeDatastorePath) asDirWithRoot(root string) string {
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

func (self UnsafeDatastorePath) AsDatastoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) +
		GetExtensionForDatastore(self, self.path_type)
}

func (self UnsafeDatastorePath) AsFilestoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsFilestoreDirectory(config_obj) +
		GetExtensionForFilestore(self, self.path_type)
}

func (self UnsafeDatastorePath) AsFilestoreDirectory(
	config_obj *config_proto.Config) string {
	return self.asDirWithRoot(config_obj.Datastore.FilestoreDirectory)
}

func NewUnsafeDatastorePath(path_components ...string) UnsafeDatastorePath {
	// For windows OS paths need to have a prefix of \\?\
	// (e.g. \\?\C:\Windows) in order to enable long file names.
	result := UnsafeDatastorePath{
		components: path_components,
		// By default write JSON files.
		path_type: PATH_TYPE_DATASTORE_JSON,
	}

	return result
}

type SafeDatastorePath struct {
	components []string
	path_type  PathType
}

func NewSafeDatastorePath(path_components ...string) SafeDatastorePath {
	// For windows OS paths need to have a prefix of \\?\
	// (e.g. \\?\C:\Windows) in order to enable long file names.
	result := SafeDatastorePath{
		components: path_components,
		path_type:  PATH_TYPE_DATASTORE_JSON,
	}

	return result
}

func (self *SafeDatastorePath) MarshalJSON() ([]byte, error) {
	return []byte(self.AsClientPath()), nil
}

// A SafeDatastorePath can be safely converted to an unsafe one. Since
// the safe path guarantees there is no need to escape any components
// (components do not contain special characters).
func (self SafeDatastorePath) AsUnsafe() PathSpec {
	return UnsafeDatastorePath{
		components: self.components,
		path_type:  self.path_type,
	}
}

func (self SafeDatastorePath) Components() []string {
	return self.components
}

func (self SafeDatastorePath) Base() string {
	return self.components[len(self.components)-1]
}

func (self SafeDatastorePath) Dir() PathSpec {
	new_components := utils.CopySlice(self.components)
	if len(new_components) > 0 {
		new_components = new_components[:len(new_components)-1]
	}

	return &SafeDatastorePath{
		components: new_components,
		path_type:  self.path_type,
	}
}

// It is a json components if it ends with a json extension.
func (self SafeDatastorePath) Type() PathType {
	return self.path_type
}

func (self SafeDatastorePath) IsEmpty() bool {
	return len(self.components) == 0
}

func (self SafeDatastorePath) SetType(t PathType) PathSpec {
	return SafeDatastorePath{
		components: self.components,
		path_type:  t,
	}
}

func (self SafeDatastorePath) AddChild(child ...string) PathSpec {
	new_components := make([]string, 0, len(self.components)+len(child))
	new_components = append(new_components, self.components...)
	new_components = append(new_components, child...)

	return SafeDatastorePath{
		components: new_components,
		path_type:  self.path_type,
	}
}

func (self SafeDatastorePath) AddUnsafeChild(child ...string) PathSpec {
	return self.AsUnsafe().AddChild(child...)
}

func (self SafeDatastorePath) AsDatastoreDirectory(
	config_obj *config_proto.Config) string {
	return self.asDirWithRoot(config_obj.Datastore.Location)
}

func (self SafeDatastorePath) AsFilestoreDirectory(
	config_obj *config_proto.Config) string {
	return self.asDirWithRoot(config_obj.Datastore.FilestoreDirectory)
}

func (self SafeDatastorePath) asDirWithRoot(root string) string {
	// No need to sanitize here because the PathSpec is already
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

func (self SafeDatastorePath) AsDatastoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) +
		GetExtensionForDatastore(self, self.path_type)
}

func (self SafeDatastorePath) AsFilestoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsFilestoreDirectory(config_obj) +
		GetExtensionForFilestore(self, self.path_type)
}

func (self SafeDatastorePath) AsClientPath() string {
	return utils.JoinComponents(self.components, "/")
}
