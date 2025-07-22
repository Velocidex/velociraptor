package api

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
	PATH_TYPE_DATASTORE_DIRECTORY
	PATH_TYPE_DATASTORE_UNKNOWN

	PATH_TYPE_FILESTORE_JSON
	PATH_TYPE_FILESTORE_JSON_INDEX
	PATH_TYPE_FILESTORE_JSON_TIME_INDEX

	// Used to write sparse indexes
	PATH_TYPE_FILESTORE_SPARSE_IDX

	// Used to write zip files in the download folder.
	PATH_TYPE_FILESTORE_DOWNLOAD_ZIP
	PATH_TYPE_FILESTORE_DOWNLOAD_REPORT

	// Used to write chunk files
	PATH_TYPE_FILESTORE_CHUNK_INDEX

	// TMP files
	PATH_TYPE_FILESTORE_TMP
	PATH_TYPE_FILESTORE_CSV

	// Used for artifacts
	PATH_TYPE_FILESTORE_YAML

	// Used to read raw db paths with the file store.  FIXME: This
	// only works when data store and file store share path - this is
	// currently only needed to read data store items with the fs
	// accessor.
	PATH_TYPE_FILESTORE_DB
	PATH_TYPE_FILESTORE_DB_JSON

	// Arbitrary extensions.
	PATH_TYPE_FILESTORE_ANY
)

func (self PathType) String() string {
	switch self {
	case PATH_TYPE_DATASTORE_JSON:
		return "PATH_TYPE_DATASTORE_JSON"

	case PATH_TYPE_DATASTORE_PROTO:
		return "PATH_TYPE_DATASTORE_PROTO"

	case PATH_TYPE_DATASTORE_DIRECTORY:
		return "PATH_TYPE_DATASTORE_DIRECTORY"

	case PATH_TYPE_DATASTORE_UNKNOWN:
		return "PATH_TYPE_DATASTORE_UNKNOWN"

	case PATH_TYPE_FILESTORE_JSON:
		return "PATH_TYPE_FILESTORE_JSON"

	case PATH_TYPE_FILESTORE_JSON_INDEX:
		return "PATH_TYPE_FILESTORE_JSON_INDEX"

	case PATH_TYPE_FILESTORE_JSON_TIME_INDEX:
		return "PATH_TYPE_FILESTORE_JSON_TIME_INDEX"

	case PATH_TYPE_FILESTORE_SPARSE_IDX:
		return "PATH_TYPE_FILESTORE_SPARSE_IDX"

	case PATH_TYPE_FILESTORE_DOWNLOAD_ZIP:
		return "PATH_TYPE_FILESTORE_DOWNLOAD_ZIP"

	case PATH_TYPE_FILESTORE_CHUNK_INDEX:
		return "PATH_TYPE_FILESTORE_CHUNK_INDEX"

	case PATH_TYPE_FILESTORE_DOWNLOAD_REPORT:
		return "PATH_TYPE_FILESTORE_DOWNLOAD_REPORT"

	case PATH_TYPE_FILESTORE_TMP:
		return "PATH_TYPE_FILESTORE_TMP"
	case PATH_TYPE_FILESTORE_CSV:
		return "PATH_TYPE_FILESTORE_CSV"

	case PATH_TYPE_FILESTORE_YAML:
		return "PATH_TYPE_FILESTORE_YAML"

	case PATH_TYPE_FILESTORE_DB:
		return "PATH_TYPE_FILESTORE_DB"

	case PATH_TYPE_FILESTORE_DB_JSON:
		return "PATH_TYPE_FILESTORE_DB_JSON"

	case PATH_TYPE_FILESTORE_ANY:
		return "PATH_TYPE_FILESTORE_ANY"
	default:
		return "Unknown PATH_TYPE"
	}
}

type _PathSpec interface {
	// A path suitable to be exchanged with the client.
	AsClientPath() string

	Components() []string
	Base() string
	Type() PathType

	String() string

	// Does any of the components need escaping?
	IsSafe() bool
}

type DSPathSpec interface {
	_PathSpec

	Dir() DSPathSpec

	SetType(t PathType) DSPathSpec

	// Adds a child maintaining safety (i.e. for a safe path keeps
	// the path safe, and for unsafe paths keep them as unsafe).
	AddChild(child ...string) DSPathSpec

	// Adds children but makes sure that the result is unsafe.
	AddUnsafeChild(child ...string) DSPathSpec

	AsFilestorePath() FSPathSpec

	Tag() string
	SetTag(string) DSPathSpec

	// If true we can apply this path to ListChildren()
	IsDir() bool
	SetDir() DSPathSpec
}

type FSPathSpec interface {
	_PathSpec

	Dir() FSPathSpec

	SetType(t PathType) FSPathSpec

	AsDatastorePath() DSPathSpec

	Tag() string
	SetTag(string) FSPathSpec

	// Adds a child maintaining safety (i.e. for a safe path keeps
	// the path safe, and for unsafe paths keep them as unsafe).
	AddChild(child ...string) FSPathSpec

	// Adds children but makes sure that the result is unsafe.
	AddUnsafeChild(child ...string) FSPathSpec
}
