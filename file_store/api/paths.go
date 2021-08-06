package api

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
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

	// Used for artifacts
	PATH_TYPE_FILESTORE_YAML

	// Arbitrary extensions.
	PATH_TYPE_FILESTORE_ANY
)

type _PathSpec interface {
	// A path suitable to be exchanged with the client.
	AsClientPath() string

	Components() []string
	Base() string
	Type() PathType
}

type DSPathSpec interface {
	_PathSpec
	AsDatastoreDirectory(config_obj *config_proto.Config) string
	AsDatastoreFilename(config_obj *config_proto.Config) string

	Dir() DSPathSpec

	SetType(t PathType) DSPathSpec

	// Adds a child maintaining safety (i.e. for a safe path keeps
	// the path safe, and for unsafe paths keep them as unsafe).
	AddChild(child ...string) DSPathSpec

	// Adds children but makes sure that the result is unsafe.
	AddUnsafeChild(child ...string) DSPathSpec

	AsFilestorePath() FSPathSpec
}

type FSPathSpec interface {
	_PathSpec
	AsFilestoreFilename(config_obj *config_proto.Config) string
	AsFilestoreDirectory(config_obj *config_proto.Config) string

	Dir() FSPathSpec

	SetType(t PathType) FSPathSpec

	// Adds a child maintaining safety (i.e. for a safe path keeps
	// the path safe, and for unsafe paths keep them as unsafe).
	AddChild(child ...string) FSPathSpec

	// Adds children but makes sure that the result is unsafe.
	AddUnsafeChild(child ...string) FSPathSpec
}
