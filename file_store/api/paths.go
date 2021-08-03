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
	Type() string

	SetType(t string) PathSpec

	AddChild(child ...string) PathSpec
	AddUnsafeChild(child ...string) PathSpec
}

type UnsafeDatastorePath struct {
	components []string
	extension  string
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
		extension:  self.extension,
	}
}

func (self UnsafeDatastorePath) Components() []string {
	return self.components
}

func (self UnsafeDatastorePath) Type() string {
	if self.extension == ".json" {
		return "json"
	}
	return ""
}

func (self UnsafeDatastorePath) AsSafe() SafeDatastorePath {
	new_components := make([]string, 0, len(self.components))
	for _, i := range self.components {
		new_components = append(new_components, utils.SanitizeString(i))
	}

	return SafeDatastorePath{
		components: new_components,
		extension:  self.extension,
	}
}

// Adds an unsafe component to this path.
func (self UnsafeDatastorePath) AddChild(child ...string) PathSpec {
	return UnsafeDatastorePath{
		components: append(utils.CopySlice(self.components), child...),
		extension:  self.extension,
	}
}

func (self UnsafeDatastorePath) AddUnsafeChild(child ...string) PathSpec {
	return self.AddChild(child...)
}

// Adds the file extension to the last component. Makes a copy of the
// slice.
func (self UnsafeDatastorePath) SetType(ext string) PathSpec {
	return UnsafeDatastorePath{
		components: self.components,
		extension:  ext,
	}
}

func (self UnsafeDatastorePath) AsClientPath() string {
	return utils.JoinComponents(self.components, "/")
}

func (self UnsafeDatastorePath) AsDatastoreDirectory(
	config_obj *config_proto.Config) string {

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
		return "\\\\?\\" + config_obj.Datastore.Location + result
	}
	return config_obj.Datastore.Location + result
}

func (self UnsafeDatastorePath) Extension() string {
	if self.extension == "" {
		return ""
	}
	return "." + self.extension
}

func (self UnsafeDatastorePath) AsDatastoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) + self.Extension() + ".db"
}

func (self UnsafeDatastorePath) AsFilestoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) + self.Extension()
}

func (self UnsafeDatastorePath) AsFilestoreDirectory(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj)
}

func NewUnsafeDatastorePath(path_components ...string) UnsafeDatastorePath {
	// For windows OS paths need to have a prefix of \\?\
	// (e.g. \\?\C:\Windows) in order to enable long file names.
	result := UnsafeDatastorePath{
		components: path_components,
		extension:  "json",
	}

	return result
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
		extension:  "json",
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
		extension:  self.extension,
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
		extension:  self.extension,
	}
}

// It is a json components if it ends with a json extension.
func (self SafeDatastorePath) Type() string {
	return self.extension
}

func (self SafeDatastorePath) IsEmpty() bool {
	return len(self.components) == 0
}

func (self SafeDatastorePath) SetType(ext string) PathSpec {
	return SafeDatastorePath{
		components: self.components,
		extension:  ext,
	}
}

func (self SafeDatastorePath) AddChild(child ...string) PathSpec {
	new_components := make([]string, 0, len(self.components)+len(child))
	new_components = append(new_components, self.components...)
	new_components = append(new_components, child...)

	return SafeDatastorePath{
		components: new_components,
	}
}

func (self SafeDatastorePath) AddUnsafeChild(child ...string) PathSpec {
	return self.AsUnsafe().AddChild(child...)
}

func (self SafeDatastorePath) AsDatastoreDirectory(
	config_obj *config_proto.Config) string {

	// No need to sanitize here because the PathSpec is already
	// safe.
	sep := string(filepath.Separator)
	result := strings.Join(self.components, sep)

	// This relies on the filepath starting with a drive letter
	// and having \ as path separators, and having a trailing
	// \. Main's config.ValidateDatastoreConfig() ensures this is
	// the case.
	if runtime.GOOS == "windows" {
		return "\\\\?\\" + config_obj.Datastore.Location + result
	}
	return config_obj.Datastore.Location + result
}

func (self SafeDatastorePath) Extension() string {
	if self.extension == "" {
		return ""
	}
	return "." + self.extension
}

func (self SafeDatastorePath) AsDatastoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) + self.Extension() + ".db"
}

func (self SafeDatastorePath) AsFilestoreFilename(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj) + self.Extension()
}

func (self SafeDatastorePath) AsFilestoreDirectory(
	config_obj *config_proto.Config) string {
	return self.AsDatastoreDirectory(config_obj)
}

func (self SafeDatastorePath) AsClientPath() string {
	return utils.JoinComponents(self.components, "/")
}

func AsDownloadURL(in PathSpec) string {
	return "/downloads/" + strings.Join(in.Components(), "/")
}
